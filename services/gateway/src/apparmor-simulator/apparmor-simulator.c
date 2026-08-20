/*
 * apparmor-simulator.c - Synthetic AppArmor event emitter for the vCPE gateway container.
 *
 * Registers with parodus via libparodus and emits WRP_MSG_TYPE__EVENT messages
 * on a configurable timer. Also registers an RBUS method for on-demand emission.
 *
 * Environment variables:
 *   APPARMOR_SIM_INTERVAL_SEC  Emission interval in seconds (default: 30)
 *
 * Build dependencies: wrp-c, libparodus, nanomsg, rbus, cjson, pthread
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <errno.h>
#include <pthread.h>
#include <ctype.h>
#include <sys/timerfd.h>
#include <sys/epoll.h>
#include <sys/stat.h>
#include <sys/socket.h>
#include <sys/un.h>

#include <cJSON.h>
#include <syslog.h>
#include <libparodus/libparodus.h>
#include <rbus.h>
#include <wrp-c.h>

#include "events.h"

/* ─── Constants ──────────────────────────────────────────────────────────── */

#define LOG_IDENT            "apparmor-simulator"
#define PARODUS_URL          "tcp://127.0.0.1:6666"
#define CLIENT_URL           "tcp://127.0.0.1:6670"
#define DEFAULT_INTERVAL_SEC 30
#define MAC_ADDR_FILE        "/sys/class/net/erouter0/address"
#define MAC_HEX_LEN          13   /* 12 hex chars + NUL */
#define RBUS_COMPONENT       "apparmor-simulator"
#define RBUS_METHOD          "Device.AppArmor.SimulateEvent()"
#define MAX_BACKOFF_SEC      60
#define DIAGNOSTIC_SOCKET    "/run/apparmor-simulator-diagnostic.sock"
#define MAX_DIAGNOSTIC_REQUEST 512

/* ─── Global state ───────────────────────────────────────────────────────── */

static libpd_instance_t libpd_instance   = NULL;
static rbusHandle_t     rbus_handle      = NULL;
static char             mac_hex[MAC_HEX_LEN];
static char             device_id[32];    /* "mac:<mac_hex>" */
static int              event_counter    = 0;
static pthread_mutex_t  counter_mutex   = PTHREAD_MUTEX_INITIALIZER;

const char* rdk_logger_module_fetch(void) {
   return "LOG.RDK.APPARMOR_SIMULATOR";
}

/* ─── 2.1 MAC address reader ─────────────────────────────────────────────── */

/*
 * read_mac_address() - reads /sys/class/net/erouter0/address, strips colons,
 * stores result in mac_hex[]. Exits with a syslog ERROR if file cannot
 * be opened (systemd Restart=always handles recovery).
 */
static void read_mac_address(void)
{
    FILE *f = fopen(MAC_ADDR_FILE, "r");
    if (!f) {
        syslog(LOG_ERR, "Cannot open %s: %s — erouter0 not yet up",
                      MAC_ADDR_FILE, strerror(errno));
        exit(1);
    }

    char raw[18] = {0};  /* "aa:bb:cc:dd:ee:ff\n" */
    if (!fgets(raw, sizeof(raw), f)) {
        syslog(LOG_ERR, "Failed to read %s", MAC_ADDR_FILE);
        fclose(f);
        exit(1);
    }
    fclose(f);

    /* Strip colons and newline */
    int j = 0;
    for (int i = 0; raw[i] && j < MAC_HEX_LEN - 1; i++) {
        if (raw[i] != ':' && raw[i] != '\n' && raw[i] != '\r') {
            mac_hex[j++] = raw[i];
        }
    }
    mac_hex[j] = '\0';
    snprintf(device_id, sizeof(device_id), "mac:%s", mac_hex);

    syslog(LOG_INFO, "Device ID: %s", device_id);
}

/* ─── 2.2 Event emission ─────────────────────────────────────────────────── */

/*
 * build_json_payload() - builds the AppArmor event JSON for the given fixture
 * index. Returns a heap-allocated string (caller must free). Returns NULL on
 * allocation failure.
 */
static char *build_json_payload(int index)
{
    const AppArmorEvent *ev = &apparmor_events[index];
    time_t now = time(NULL);
    struct tm *tm_info = gmtime(&now);
    char ts[32];
    strftime(ts, sizeof(ts), "%Y-%m-%dT%H:%M:%SZ", tm_info);

    int pid = (getpid() + index) % 65535;

    cJSON *root = cJSON_CreateObject();
    if (!root) return NULL;

    cJSON_AddStringToObject(root, "timestamp",      ts);
    cJSON_AddStringToObject(root, "device_id",      device_id);
    cJSON_AddStringToObject(root, "apparmor",       ev->apparmor);
    cJSON_AddStringToObject(root, "operation",      ev->operation);
    cJSON_AddStringToObject(root, "profile",        ev->profile);
    cJSON_AddStringToObject(root, "name",           ev->name);
    cJSON_AddNumberToObject(root, "pid",            pid);
    cJSON_AddStringToObject(root, "comm",           ev->comm);
    cJSON_AddStringToObject(root, "requested_mask", ev->requested_mask);
    cJSON_AddStringToObject(root, "denied_mask",    ev->denied_mask);
    cJSON_AddNumberToObject(root, "fsuid",          ev->fsuid);
    cJSON_AddNumberToObject(root, "ouid",           ev->ouid);
    cJSON_AddBoolToObject(root,   "simulated",      cJSON_True);

    char *payload = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);
    return payload;
}

/*
 * emit_apparmor_event() - constructs and sends one WRP_MSG_TYPE__EVENT.
 * Logs WARN if libparodus_send fails; never blocks the caller.
 */
static void emit_apparmor_event(int index)
{
    index = index % FIXTURE_COUNT;
    const AppArmorEvent *ev = &apparmor_events[index];

    char *payload = build_json_payload(index);
    if (!payload) {
        syslog(LOG_ERR, "Failed to build JSON payload for index %d", index);
        return;
    }

    /* Build WRP source and dest strings */
    char source[64];
    char dest[64];
    snprintf(source, sizeof(source), "%s/apparmor-simulator", device_id);
    snprintf(dest,   sizeof(dest),   "event:apparmor/%s/%s", ev->severity, device_id);

    /* Allocate and populate WRP event message */
    wrp_msg_t *msg = calloc(1, sizeof(wrp_msg_t));
    if (!msg) {
        syslog(LOG_ERR, "calloc failed for WRP message");
        free(payload);
        return;
    }

    msg->msg_type                = WRP_MSG_TYPE__EVENT;
    msg->u.event.source          = strdup(source);
    msg->u.event.dest            = strdup(dest);
    msg->u.event.content_type    = strdup("application/json");
    msg->u.event.payload         = payload;  /* libparodus takes ownership */
    msg->u.event.payload_size    = strlen(payload);

    int ret = libparodus_send(libpd_instance, msg);
    if (ret != 0) {
        syslog(LOG_ERR,
                     "libparodus_send failed (ret=%d): dest=%s op=%s",
                     ret, dest, ev->operation);
    } else {
        syslog(LOG_INFO, "Emitted event: dest=%s op=%s profile=%s",
                     dest, ev->operation, ev->profile);
    }

    wrp_free_struct(msg);
}

static int is_stable_id(const char *value, size_t max_len)
{
    if (!value || value[0] == '\0' || strlen(value) > max_len || !islower((unsigned char)value[0]) && !isdigit((unsigned char)value[0])) return 0;
    for (size_t index = 0; value[index]; index++) {
        if (!islower((unsigned char)value[index]) && !isdigit((unsigned char)value[index]) && value[index] != '.' && value[index] != '_' && value[index] != '-') return 0;
    }
    return 1;
}

static int is_event_destination(const char *value)
{
    if (!value || value[0] == '\0' || strlen(value) > 256 || !islower((unsigned char)value[0]) && !isdigit((unsigned char)value[0])) return 0;
    for (size_t index = 0; value[index]; index++) {
        if (!islower((unsigned char)value[index]) && !isdigit((unsigned char)value[index]) && value[index] != '.' && value[index] != '_' && value[index] != '-' && value[index] != '/') return 0;
    }
    return 1;
}

static int is_correlation_id(const char *value)
{
    if (!value || strlen(value) != 64) return 0;
    for (size_t index = 0; index < 64; index++) {
        if (!isdigit((unsigned char)value[index]) && (value[index] < 'a' || value[index] > 'f')) return 0;
    }
    return 1;
}

static int emit_diagnostic_event(const char *event, const char *correlation_id)
{
    char destination[320];
    char source[64];
    snprintf(destination, sizeof(destination), "event:%s/%s", event, device_id);
    snprintf(source, sizeof(source), "%s/apparmor-simulator", device_id);

    cJSON *marker = cJSON_CreateObject();
    if (!marker) {
        syslog(LOG_ERR, "Failed to allocate diagnostic marker");
        return -1;
    }
    cJSON_AddStringToObject(marker, "vcpe_diagnostic", "cpe-webpa-callback");
    cJSON_AddStringToObject(marker, "correlation_id", correlation_id);
    char *payload = cJSON_PrintUnformatted(marker);
    cJSON_Delete(marker);
    if (!payload) {
        syslog(LOG_ERR, "Failed to encode diagnostic marker");
        return -1;
    }

    wrp_msg_t *msg = calloc(1, sizeof(wrp_msg_t));
    if (!msg) {
        syslog(LOG_ERR, "Failed to allocate diagnostic WRP message");
        free(payload);
        return -1;
    }
    msg->msg_type = WRP_MSG_TYPE__EVENT;
    msg->u.event.source = strdup(source);
    msg->u.event.dest = strdup(destination);
    msg->u.event.content_type = strdup("application/json");
    msg->u.event.payload = payload;
    msg->u.event.payload_size = strlen(payload);
    int result = libparodus_send(libpd_instance, msg);
    if (result == 0) {
        syslog(LOG_INFO, "Accepted diagnostic event: dest=%s", destination);
    } else {
        syslog(LOG_ERR, "Rejected diagnostic event: libparodus_send ret=%d dest=%s", result, destination);
    }
    wrp_free_struct(msg);
    return result;
}

static int setup_diagnostic_socket(void)
{
    int fd = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (fd < 0) return -1;
    struct sockaddr_un address = { .sun_family = AF_UNIX };
    strncpy(address.sun_path, DIAGNOSTIC_SOCKET, sizeof(address.sun_path) - 1);
    unlink(DIAGNOSTIC_SOCKET);
    if (bind(fd, (struct sockaddr *)&address, sizeof(address)) != 0 || chmod(DIAGNOSTIC_SOCKET, 0600) != 0 || listen(fd, 4) != 0) {
        close(fd);
        unlink(DIAGNOSTIC_SOCKET);
        return -1;
    }
    return fd;
}

static void handle_diagnostic_request(int listen_fd)
{
    int fd = accept(listen_fd, NULL, NULL);
    if (fd < 0) return;
    char body[MAX_DIAGNOSTIC_REQUEST + 1] = {0};
    ssize_t length = read(fd, body, MAX_DIAGNOSTIC_REQUEST);
    int accepted = 0;
    if (length > 0 && length < MAX_DIAGNOSTIC_REQUEST) {
        cJSON *request = cJSON_ParseWithLength(body, (size_t)length);
        if (request && cJSON_IsObject(request) && cJSON_GetArraySize(request) == 4) {
            cJSON *client = cJSON_GetObjectItemCaseSensitive(request, "clientService");
            cJSON *event = cJSON_GetObjectItemCaseSensitive(request, "event");
            cJSON *device = cJSON_GetObjectItemCaseSensitive(request, "deviceId");
            cJSON *correlation = cJSON_GetObjectItemCaseSensitive(request, "correlationId");
            if (cJSON_IsString(client) && cJSON_IsString(event) && cJSON_IsString(device) && cJSON_IsString(correlation) &&
                is_stable_id(client->valuestring, 64) && strcmp(client->valuestring, "apparmor-simulator") == 0 &&
                is_event_destination(event->valuestring) && strcmp(device->valuestring, device_id) == 0 &&
                is_correlation_id(correlation->valuestring)) {
                accepted = emit_diagnostic_event(event->valuestring, correlation->valuestring) == 0;
            }
        }
        cJSON_Delete(request);
    }
    if (!accepted) {
        syslog(LOG_WARNING, "Rejected diagnostic event request");
    }
    (void)write(fd, accepted ? "accepted\n" : "rejected\n", accepted ? 9 : 9);
    close(fd);
}

/* ─── 2.4 RBUS method handler ────────────────────────────────────────────── */

static rbusError_t apparmor_simulate_method(
        rbusHandle_t handle,
        char const *methodName,
        rbusObject_t inParams,
        rbusObject_t outParams,
        rbusMethodAsyncHandle_t asyncHandle)
{
    (void)handle; (void)methodName;
    (void)inParams; (void)outParams; (void)asyncHandle;

    int idx;
    pthread_mutex_lock(&counter_mutex);
    idx = event_counter++;
    pthread_mutex_unlock(&counter_mutex);

    syslog(LOG_INFO, "RBUS on-demand trigger received");
    emit_apparmor_event(idx % FIXTURE_COUNT);
    return RBUS_ERROR_SUCCESS;
}

/* ─── RBUS init ──────────────────────────────────────────────────────────── */

static void init_rbus(void)
{
    rbusError_t rc = rbus_open(&rbus_handle, RBUS_COMPONENT);
    if (rc != RBUS_ERROR_SUCCESS) {
        syslog(LOG_ERR, "rbus_open failed (rc=%d): RBUS method unavailable", rc);
        rbus_handle = NULL;
        return;
    }

    rbusDataElement_t elements[1] = {{
        .name = RBUS_METHOD,
        .type = RBUS_ELEMENT_TYPE_METHOD,
        .cbTable = { .methodHandler = apparmor_simulate_method }
    }};

    rc = rbus_regDataElements(rbus_handle, 1, elements);
    if (rc != RBUS_ERROR_SUCCESS) {
        syslog(LOG_ERR,
                     "rbus_regDataElements failed (rc=%d): RBUS method unavailable", rc);
        rbus_close(rbus_handle);
        rbus_handle = NULL;
        return;
    }

    syslog(LOG_INFO, "Registered RBUS method: %s", RBUS_METHOD);
}

/* ─── 2.5 libparodus init with retry ─────────────────────────────────────── */

static void init_parodus(void)
{
    libpd_cfg_t cfg = {
        .service_name         = "apparmor-simulator",
        .receive              = true,
        .keepalive_timeout_secs = 60,
        .parodus_url          = PARODUS_URL,
        .client_url           = CLIENT_URL,
    };

    int delay = 1;
    while (1) {
        int ret = libparodus_init(&libpd_instance, &cfg);
        if (ret == 0) {
            syslog(LOG_INFO, "Registered with parodus at %s", PARODUS_URL);
            return;
        }
        syslog(LOG_ERR,
                      "libparodus_init failed (ret=%d), retrying in %ds", ret, delay);
        sleep(delay);
        if (delay < MAX_BACKOFF_SEC) {
            delay = delay * 2;
            if (delay > MAX_BACKOFF_SEC) delay = MAX_BACKOFF_SEC;
        }
    }
}

/* ─── Receive drain thread ───────────────────────────────────────────────── */
/*
 * parodus sends keepalive pings to the client URL. We must drain the receive
 * queue to avoid backpressure. Messages are discarded — this service only sends.
 */
static void *receive_drain_thread(void *arg)
{
    (void)arg;
    wrp_msg_t *msg = NULL;
    while (1) {
        int ret = libparodus_receive(libpd_instance, &msg, 2000);
        if (ret == 0 && msg) {
            wrp_free_struct(msg);
            msg = NULL;
        }
    }
    return NULL;
}

/* ─── 2.3 timerfd event loop (main) ─────────────────────────────────────── */

int main(void)
{
    openlog(LOG_IDENT, LOG_PID | LOG_CONS, LOG_DAEMON);
    syslog(LOG_INFO, "apparmor-simulator starting");

    /* 2.1 Read device MAC */
    read_mac_address();

    /* Read interval from env */
    int interval_sec = DEFAULT_INTERVAL_SEC;
    const char *env_interval = getenv("APPARMOR_SIM_INTERVAL_SEC");
    if (env_interval) {
        int parsed = atoi(env_interval);
        if (parsed > 0) {
            interval_sec = parsed;
        } else {
            syslog(LOG_ERR,
                         "Invalid APPARMOR_SIM_INTERVAL_SEC='%s', using default %d",
                         env_interval, DEFAULT_INTERVAL_SEC);
        }
    }
    syslog(LOG_INFO, "Emission interval: %ds", interval_sec);

    /* Init RBUS (optional; failure is non-fatal) */
    init_rbus();

    /* Init parodus (blocks until connected) */
    init_parodus();

    /* Start receive drain thread */
    pthread_t drain_tid;
    if (pthread_create(&drain_tid, NULL, receive_drain_thread, NULL) != 0) {
        syslog(LOG_ERR, "Failed to create receive drain thread: %s",
                     strerror(errno));
    } else {
        pthread_detach(drain_tid);
    }

    /* Create timerfd */
    int tfd = timerfd_create(CLOCK_MONOTONIC, TFD_NONBLOCK);
    if (tfd < 0) {
        syslog(LOG_ERR, "timerfd_create failed: %s", strerror(errno));
        exit(1);
    }

    struct itimerspec its = {
        .it_interval = { .tv_sec = interval_sec, .tv_nsec = 0 },
        .it_value    = { .tv_sec = interval_sec, .tv_nsec = 0 },
    };
    if (timerfd_settime(tfd, 0, &its, NULL) < 0) {
        syslog(LOG_ERR, "timerfd_settime failed: %s", strerror(errno));
        exit(1);
    }

    /* Set up epoll */
    int efd = epoll_create1(0);
    if (efd < 0) {
        syslog(LOG_ERR, "epoll_create1 failed: %s", strerror(errno));
        exit(1);
    }

    struct epoll_event ev = { .events = EPOLLIN, .data.fd = tfd };
    epoll_ctl(efd, EPOLL_CTL_ADD, tfd, &ev);

    int diagnostic_fd = setup_diagnostic_socket();
    if (diagnostic_fd < 0) {
        syslog(LOG_ERR, "Failed to create diagnostic event socket: %s", strerror(errno));
    } else {
        ev.data.fd = diagnostic_fd;
        epoll_ctl(efd, EPOLL_CTL_ADD, diagnostic_fd, &ev);
    }

    syslog(LOG_INFO, "Event loop started (interval=%ds)", interval_sec);

    /* Main event loop */
    struct epoll_event events[2];
    while (1) {
        int n = epoll_wait(efd, events, 1, -1);
        if (n < 0) {
            if (errno == EINTR) continue;
            syslog(LOG_ERR, "epoll_wait error: %s", strerror(errno));
            break;
        }
        if (n > 0 && events[0].data.fd == diagnostic_fd && (events[0].events & EPOLLIN)) {
            handle_diagnostic_request(diagnostic_fd);
        } else if (n > 0 && (events[0].events & EPOLLIN)) {
            uint64_t expirations = 0;
            read(tfd, &expirations, sizeof(expirations));

            int idx;
            pthread_mutex_lock(&counter_mutex);
            idx = event_counter;
            event_counter += (int)expirations;
            pthread_mutex_unlock(&counter_mutex);

            emit_apparmor_event(idx % FIXTURE_COUNT);
        }
    }

    /* Cleanup (not reached in normal operation) */
    if (diagnostic_fd >= 0) {
        close(diagnostic_fd);
        unlink(DIAGNOSTIC_SOCKET);
    }
    if (rbus_handle) rbus_close(rbus_handle);
    return 0;
}

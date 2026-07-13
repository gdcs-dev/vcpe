#pragma once

/*
 * events.h - AppArmor event fixture type and table declaration.
 *
 * The fixture table contains FIXTURE_COUNT pre-defined synthetic AppArmor
 * audit events drawn from realistic gateway-resident process profiles.
 * The simulator cycles through them in order (index % FIXTURE_COUNT).
 */

#define FIXTURE_COUNT 12

typedef struct {
    const char *profile;         /* AppArmor profile path */
    const char *operation;       /* kernel operation: open, exec, connect, ... */
    const char *name;            /* resource name */
    const char *comm;            /* process basename */
    const char *apparmor;        /* "DENIED", "AUDIT", or "ALLOWED" */
    const char *severity;        /* lowercase: "denied", "audit", "allowed" */
    const char *requested_mask;  /* requested access mask */
    const char *denied_mask;     /* denied access mask */
    int         fsuid;           /* filesystem UID */
    int         ouid;            /* object owner UID */
} AppArmorEvent;

extern const AppArmorEvent apparmor_events[FIXTURE_COUNT];

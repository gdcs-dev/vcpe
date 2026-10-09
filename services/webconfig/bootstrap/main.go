package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rdkcentral/webconfig/common"
	"github.com/rdkcentral/webconfig/db"
	"github.com/rdkcentral/webconfig/db/sqlite"
	"github.com/rdkcentral/webconfig/util"
	"github.com/vmihailenco/msgpack"
)

const defaultGatewayMAC = "02:00:00:00:00:01"

// Fixture agent.yaml delivered as the seeded "otelagent" subdoc. Structurally
// valid per otelagent-webconfig-bridge's validate.c (node.client_id non-empty,
// node.otlp.endpoint starting with "http", node.otlp.bearer_credential_path
// starting with "/", collectors a sequence).
const otelAgentYAMLFixture = `node:
  client_id: vcpe-gateway
  topic_prefix: rdk
  server_url: http://127.0.0.1:4318
  otlp:
    endpoint: http://127.0.0.1:4318/v1/metrics
    bearer_credential_path: /run/rdk-otel/agent-token
collectors:
  - topic: mem_info
    source: {kind: file, path: /proc/meminfo}
    parser: kv_colon
    kb_to_bytes: true
`

func main() {
	configFile := flag.String("f", "/etc/webconfig/webconfig.conf", "WebConfig HOCON file")
	flag.Parse()

	serverConfig, err := common.NewServerConfig(*configFile)
	failIf(err)
	client, err := sqlite.NewSqliteClient(serverConfig.Config, false)
	failIf(err)
	failIf(client.SetUp())
	failIf(client.SyncSchema())

	mac := strings.ToUpper(strings.TrimSpace(os.Getenv("WEBCONFIG_GATEWAY_MAC")))
	if mac == "" {
		mac = defaultGatewayMAC
	}

	payload, err := fixturePayload()
	failIf(err)
	version := util.GetMurmur3Hash(payload)
	state := common.PendingDownload
	updatedTime := 0
	document := common.NewSubDocument(payload, &version, &state, &updatedTime, nil, nil)
	failIf(client.SetSubDocument(mac, "privatessid", document))

	otelAgentPayload, err := otelAgentFixturePayload()
	failIf(err)
	otelAgentVersion := util.GetMurmur3Hash(otelAgentPayload)
	otelAgentState := common.PendingDownload
	otelAgentUpdatedTime := 0
	otelAgentDocument := common.NewSubDocument(otelAgentPayload, &otelAgentVersion, &otelAgentState, &otelAgentUpdatedTime, nil, nil)
	failIf(client.SetSubDocument(mac, "otelagent", otelAgentDocument))

	fullDocument, err := client.GetDocument(mac)
	failIf(err)
	failIf(client.SetRootDocumentVersion(mac, db.HashRootVersion(fullDocument.VersionMap())))
}

func fixturePayload() ([]byte, error) {
	privateWiFi, err := msgpack.Marshal(common.EmbeddedPrivateWifi{
		Ssid2g: &common.EmbeddedSsid{
			Ssid:      "vcpe-private",
			Enable:    true,
			Broadcast: true,
		},
	})
	if err != nil {
		return nil, err
	}
	return msgpack.Marshal(common.TR181Output{Parameters: []common.TR181Entry{{
		Name:     common.TR181NamePrivatessid,
		Value:    string(privateWiFi),
		DataType: common.TR181Str,
	}}})
}

// otelAgentFixturePayload builds the wire format the on-device webconfig
// client's isRbusListener/blob path expects: an inner {"agent_yaml": "..."}
// map delivered as a WDMP_BLOB-typed TR181 parameter named "otelagent".
func otelAgentFixturePayload() ([]byte, error) {
	inner, err := msgpack.Marshal(map[string]string{"agent_yaml": otelAgentYAMLFixture})
	if err != nil {
		return nil, err
	}
	return msgpack.Marshal(common.TR181Output{Parameters: []common.TR181Entry{{
		Name:     "otelagent",
		Value:    string(inner),
		DataType: common.TR181Blob,
	}}})
}

func failIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
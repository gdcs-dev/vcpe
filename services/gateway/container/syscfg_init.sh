#!/bin/bash

touch /tmp/syscfg.db
/usr/bin/syscfg_create -f /tmp/syscfg.db
/usr/bin/syscfg set T2ReportProfileConfigURL http://xconfwebconfig:9000/loguploader/getTelemetryProfiles


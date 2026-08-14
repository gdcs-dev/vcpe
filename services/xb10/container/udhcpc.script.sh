#!/bin/sh

# udhcpc script edited by Tim Riker <Tim@Rikers.org>

source /etc/utopia/service.d/log_env_var.sh
source /etc/utopia/service.d/log_capture_path.sh
source /etc/device.properties
source /etc/log_timestamp.sh

if [ "$VOICE_SUPPORTED" != "false" ]; then
mta_dhcp_option_received=0
fi
echo "****************** udhcpc.script called*************"
[ -z "$1" ] && echo "Error: should be called from udhcpc" && exit 1

RESOLV_CONF="/etc/resolv.conf"
RESOLV_CONF_TMP="/tmp/resolv_tmp.conf"
UPSTREAM_RESOLV_CONF="/etc/upstream-resolv.conf"
over_ride_RESOLV_CONF=0
upstream_dns="$dns"
[ -n "$LAN_DNS" ] && dns="$LAN_DNS"
[ -n "$broadcast" ] && BROADCAST="broadcast $broadcast"
[ -n "$subnet" ] && NETMASK="netmask $subnet"

# return 0 if root is mounted on a network filesystem
root_is_nfs() {
        sed -n 's/^[^ ]* \([^ ]*\) \([^ ]*\) .*$/\1 \2/p' /proc/mounts |
        grep -q "^/ \(nfs\|smbfs\|ncp\|coda\)$"
}

dump_dhcp_offer() {
        echo "interface     : $interface"
        echo "ip address    : $ip"
        echo "subnet mask   : $subnet"
        echo "broadcast     : $broadcast"
        echo "lease time    : $lease"
        echo "router        : $router"
        echo "hostname      : $hostname"
        echo "domain        : $domain"
        echo "next server   : $siaddr"
        echo "server name   : $sname"
        echo "server id     : $serverid"
        echo "tftp server   : $tftp"
        echo "timezone      : $timezone"
        echo "time server   : $timesvr"
        echo "name server   : $namesvr"
        echo "ntp server    : $ntpsvr"
        echo "dns server    : $dns"
        echo "wins server   : $wins"
        echo "log server    : $logsvr"
        echo "cookie server : $cookiesvr"
        echo "print server  : $lprsvr"
        echo "swap server   : $swapsvr"
        echo "boot file     : $boot_file"
        echo "boot file name: $bootfile"
        echo "bootsize      : $bootsize"
        echo "root path     : $rootpath"
        echo "ip ttl        : $ipttl"
        echo "mtu           : $mtuipttl"
        echo "vendorspecific: $vendorspecific"
}

#compare and delete old domain and ipv4 config from resolv.conf
compare_and_remove_old_config() {

                cp $RESOLV_CONF $RESOLV_CONF_TMP
                echo "comapring old and new dns IPV4 configuration " >> $CONSOLEFILE
                over_ride_RESOLV_CONF=0
                for i in $dns; do
                        new_ipv4_dns_server="nameserver $i"
                        dns_exist=`grep "$new_ipv4_dns_server" "$RESOLV_CONF_TMP"`
                        if [ "$dns_exist" = "" ]; then
                                echo "$new_ipv4_dns_server is not present in old dns config so resolv_conf file overide is required " >> $CONSOLEFILE
                                over_ride_RESOLV_CONF=1
                                break
                        fi
                done


        if [ "$over_ride_RESOLV_CONF" -eq 1 ]; then
        get_dns_number=`sysevent get ipv4_${interface}_dns_number`
        sed -i '/domain/d' "$RESOLV_CONF_TMP"
        sed -i '/nameserver 127.0.0.1/d' "$RESOLV_CONF_TMP"
        echo "WanFailOverSupportEnable:$WanFailOverSupportEnable";
        if [ "$WanFailOverSupportEnable" = true ]; then
                get_lan_ipaddr=`syscfg get lan_ipaddr`
                lan_ipaddr="nameserver $get_lan_ipaddr"
                echo "lan_ipaddr:$lan_ipaddr";
                sed -i "/$lan_ipaddr/d" "$RESOLV_CONF_TMP"
        fi
        if [ "$get_dns_number" != "" ]; then
        echo "Removing old DNS IPV4 SERVER configuration from resolv.conf " >> $CONSOLEFILE
        counter=0;
        while [ $counter -lt $get_dns_number ]; do
                get_old_dns_server=`sysevent get ipv4_${interface}_dns_$counter`
                ipv4_dns_server="nameserver $get_old_dns_server"
                sed -i "/$ipv4_dns_server/d" "$RESOLV_CONF_TMP"
                let counter=counter+1
        done
        fi
        else
                echo "old and new IPV4 dns config are same no resolv_conf file override required " >> $CONSOLEFILE
        fi
}

# save dhcp offer
save_dhcp_offer() {
        dump_dhcp_offer

        if [ x"0" = x`syscfg get staticdns_enable` ];then
            if [ "$dns" != "" ]; then
                compare_and_remove_old_config
            fi
        else
            echo "Static dns is enabled, configuring static dns"
            sh /etc/utopia/service.d/set_resolv_conf.sh
        fi
        
        set_tosysevent $1

        if [ "x$WAN_TYPE" == "xEPON" ]; then
                if [ $1 = 'bound' ]; then
                    sysevent set wan_fsm_state 5
                fi

                if [ $1 = 'renew' ]; then
                    sysevent set wan_fsm_state 6
                fi

        fi

        dns_n=0
        for i in $dns; do
                echo "===$i==="
                sysevent set ipv4_${interface}_dns_${dns_n} "$i"
                let dns_n=$dns_n+1
        done
        echo $dns_n
        sysevent set ipv4_${interface}_dns_number "${dns_n}"

        gw_n=0
        for i in $router; do
                echo "===$i==="
        if [ "x$WAN_TYPE" == "xEPON" ]; then
                sysevent set wan_gw $i
        fi
                sysevent set default_router $i
                sysevent set ipv4_${interface}_gw_${gw_n} "$i"

                let gw_n=$gw_n+1
        done
        echo $gw_n
        sysevent set ipv4_${interface}_gw_number "${gw_n}"

    ## test
        sysevent get ipv4_${interface}_subnet
        sysevent get ipv4_${interface}_ipaddr
        sysevent get ipv4_${interface}_lease_time
        sysevent get ipv4_${interface}_dhcp_server
        sysevent get ipv4_${interface}_dhcp_state
        sysevent get ipv4_${interface}_start_time
        sysevent get ipv4_${interface}_dns_number
        sysevent get ipv4_${interface}_gw_number

        idx=0
        while [[ $idx -lt $dns_n ]]; do
                sysevent get ipv4_${interface}_dns_${idx}
                true $((idx=idx+1))
        done

        idx=0
        while [[ $idx -lt $gw_n ]]; do
                sysevent get ipv4_${interface}_gw_${idx}
                true $((idx=idx+1))
        done
}

convertval()
{

    IP_MODE=$2
    LEN=$3
    # For ipv4 we need to convert value into decimal
    if [ "$IP_MODE" = "v4" ];then
        APPEND_TO_VAL="."
        hex=`echo $1 | sed "s/.\{$LEN\}/& /g"`
        for i in $hex
        do
                dec_val=`echo -en $((16#$i))`
                formatted_val="$formatted_val""$dec_val""$APPEND_TO_VAL"

        done
    else
        # Add : after 4 characters to get the ipv6 address
        formatted_val=`echo $1 | sed "s/.\{$LEN\}/&:/g"`
    fi
    echo "$formatted_val" | sed 's/.$//'

    #echo "${bkup::-1}"
}


parse_dhcp_option()
{

    dhcp_option_val=$1
    dhcp_option_val=`echo "${dhcp_option_val//:}"`
    # to count number of characters in suboption value in a dhcp option
    VAL=2

    OPTION_FORMAT=""
    IP_MODE=$2
    if [ "$IP_MODE" = "v4" ]; then
        LEN=2
    else
        LEN=4
    fi

    EQUAL="="
    PRINT_FROM_INDEX=$((LEN+1))
    while [ "$dhcp_option_val" != "" ]
    do
        SUBOPTION=`echo $dhcp_option_val | awk '{print substr ($0, 0, LEN)}' LEN=$LEN`

        dhcp_option_val=`echo "$dhcp_option_val" | awk '{print substr($0,CUR)}' CUR=$PRINT_FROM_INDEX`
        SUBOPTION_LENGTH=`echo $dhcp_option_val | awk '{print substr ($0, 0, LEN)}' LEN=$LEN`
        dhcp_option_val=`echo "$dhcp_option_val" | awk '{print substr($0,CUR)}' CUR=$PRINT_FROM_INDEX`
        SUBOPTION_LENGTH=`echo $((16#$SUBOPTION_LENGTH))`
        #SUBOPTION_LENGTH=`echo "ibase=16; $SUBOPTION_LENGTH" | bc`

        LENGTH=`echo $(($SUBOPTION_LENGTH * $VAL))`
        SUBOPTION_VALUE=`echo "$dhcp_option_val" | awk '{print substr ($0, 0, v1)}' v1=$LENGTH`
#        SUBOPTION_VALUE=`convertval $SUBOPTION_VALUE $IP_MODE $LEN`
        if [ "$OPTION_FORMAT" = "" ];then
                OPTION_FORMAT="$SUBOPTION""$EQUAL""$SUBOPTION_VALUE"
        else
                OPTION_FORMAT="$OPTION_FORMAT"" ""$SUBOPTION""$EQUAL""$SUBOPTION_VALUE"
        fi

        SUBOPTION_LENGTH=$((LENGTH+1))
        dhcp_option_val=`echo "$dhcp_option_val" | cut -c "$SUBOPTION_LENGTH"-`
     done

     echo "$OPTION_FORMAT"

}

have_bin_ip=0
if [ -x /sbin/ip.iproute2 ]; then
  have_bin_ip=1
fi

get_fromsysevent()
{
    interface=`sysevent get ipv4_interface_name`
    ip=`sysevent get ipv4_${interface}_ipaddr`
    mask=`sysevent get ipv4_${interface}_subnet`
    lease=`sysevent get ipv4_${interface}_lease_time`
    serverid=`sysevent get ipv4_${interface}_dhcp_server`
    router=`sysevent get ipv4_router`
    opt58=`sysevent get ipv4_opt58`
    opt59=`sysevent get ipv4_opt59`
    opt100=`sysevent get ipv4_opt100`
    opt122=`sysevent get ipv4_opt122`
    opt125=`sysevent get ipv4_opt125`
    timezone=`sysevent get ipv4_timezone`
    dns=`sysevent get ipv4_dns`
    domain=`sysevent get ipv4_domain`
}

set_tosysevent()
{
    sysevent set ipv4_interface_name "$interface"
    sysevent set ipv4_${interface}_ipaddr "$ip"
    sysevent set ipv4_${interface}_subnet "$mask"
    sysevent set ipv4_${interface}_lease_time "$lease"
    sysevent set ipv4_${interface}_dhcp_server "$serverid"
    sysevent set ipv4_${interface}_dhcp_state "$1"
    sysevent set ipv4_${interface}_start_time `cut -d. -f1 /proc/uptime`
    sysevent set ipv4_router "$router"
    sysevent set ipv4_opt58 "$opt58"
    sysevent set ipv4_opt59 "$opt59"
    sysevent set ipv4_opt100 "$opt100"
    sysevent set ipv4_opt122 "$opt122"
    sysevent set ipv4_opt125 "$opt125"
    sysevent set ipv4_timezone "$timezone"
    sysevent set ipv4_dns "$dns"
    sysevent set ipv4_domain "$domain"
}

apply_deconfigurations()
{
    route_val=`sysevent get ipv4_router`
    p_ifname=`sysevent get ipv4_interface_name`
    if [ $have_bin_ip -eq 1 ]; then
        ip route del default via $route_val dev $p_ifname
    else
        route del default via $route_val dev $p_ifname
    fi
}

apply_configurations()
{
    if [ $have_bin_ip -eq 1 ]; then
        if [ "$1" = "enable" ] ; then
            ip addr add dev $interface $ip/$mask $BROADCAST
        else
            get_fromsysevent
        fi
        echo "***** IP is $ip and mask is $mask  ***********"
        sysevent set ipv4_wan_subnet $mask
        sysevent set ipv4_wan_ipaddr $ip
        sysevent set current_ipv4_link_state up
        if [ "x$WAN_TYPE" == "xEPON" ]; then
            if [ "xup" != "x`sysevent get ipv4-status`" ];then
                sysevent set ipv4-status up
            fi
            sysevent set wan_ipaddr $ip
            sysevent set current_wan_ipaddr $ip

            if [ "$1" = "enable" ] ; then
                if [ "xstarted" != "x`sysevent get wan_service-status`" ];then
                    sysevent set wan_service-status started
                fi

                if [ "xstarted" != "x`sysevent get wan-status`" ];then
                    sysevent set wan-status started
                fi
            fi
            uptime=`cat /proc/uptime | awk '{ print $1 }' | cut -d"." -f1`
            touch /tmp/wan_ready
            print_uptime "Waninit_complete"
            print_uptime "boot_to_wan_uptime"
        fi
    else
        if [ "$1" = "enable" ] ; then
            /sbin/ifconfig $interface $ip $BROADCAST $NETMASK
        fi
    fi

    DEVICE_MODE=$(syscfg get Device_Mode)
    # For device types (such as XB3s) where both CM (wan0) and WAN (eoruter0) interfaces are present
    # we dont want to recreate the default wan0 route, since wan0 interfce is not controlled by udhcpc.
    # So exluding the below code for XB3 device types.
    if [ "1" != "$DEVICE_MODE" ] ; then
        if [ $BOX_TYPE != "XB3" ] ; then
            if [ -n "$router" ] ; then
                if ! root_is_nfs ; then
                    if [ $have_bin_ip -eq 1 ]; then
                        while ip route del default 2>/dev/null ; do
                        :
                        done
                    else
                        while route del default gw 0.0.0.0 dev $interface 2>/dev/null ; do
                        :
                        done
                    fi
                fi
                metric=0
                for i in $router ; do
                    if [ $have_bin_ip -eq 1 ]; then
                        ip route add default via $i metric $metric
                    else
                        route add default gw $i dev $interface metric $metric 2>/dev/null
                    fi
                    metric=$(($metric + 1))
                done
            fi
        fi
    fi

    if [ $BOX_TYPE == "XB3" ] || [ $MODEL_NUM == "TG3482G" ] ; then
        prev_ip=`sysevent get previous_wan_ipaddr`
        echo "removing ip rule based on prev_ip:$prev_ip and adding ip:$ip"
        if [ $prev_ip != "" && $prev_ip != "0.0.0.0" ]; then
            ip -4 rule del from $prev_ip lookup erouter
            ip -4 rule del from $prev_ip lookup all_lans
        fi
        ip -4 rule add from $ip lookup erouter
        ip -4 rule add from $ip lookup all_lans
    fi

    if [ -n "$serverid" ]; then
        sysevent set wan_dhcp_svr $serverid
    fi
    if [ -n "$lease" ]; then
        sysevent set wan_lease_time $lease
        lease_date=$(date +"%Y.%m.%d-%T")
        lease_expiry=$(date -d"$lease_date:$lease" +"%Y.%m.%d-%T %Z")
        sysevent set wan_lease_expiry "$lease_expiry"
    fi
    if [ -n "$opt58" ]; then
        sysevent set wan_renew_time $opt58
        lease_date=$(date +"%Y.%m.%d-%T")
        renew_time=$((0x$opt58))
        lease_renew=$(date -d"$lease_date:$renew_time" +"%Y.%m.%d-%T %Z")
        sysevent set wan_lease_renew "$lease_renew"
    fi
    if [ -n "$opt59" ]; then
        sysevent set wan_rebind_time $opt59
        lease_date=$(date +"%Y.%m.%d-%T")
        bind_time=$((0x$opt59))
        lease_bind=$(date -d"$lease_date:$bind_time" +"%Y.%m.%d-%T %Z")
        sysevent set wan_lease_rebind "$lease_bind"
    fi
    if [ -n "$subnet" ]; then
        sysevent set wan_mask $subnet
    fi
    if [ -n "$opt100" ]; then
        sysevent set ipv4_timezone $opt100
    fi

if [ "$VOICE_SUPPORTED" != "false" ]; then
    if [ -n "$opt122" ]; then
        echo "MTA DHCPv4 address received for OPT122 is $opt122"
        parsed_value=""
        parsed_value=`parse_dhcp_option $opt122 v4`
        echo "OPT122 parsed value is $parsed_value"

        suboption=""
        suboption_data=""
        for val in $parsed_value; do
            suboption=`echo $val | cut -d = -f 1`
            suboption_data=`echo $val | cut -d = -f 2`
            case "$suboption" in
                "01")
                    echo "Suboption is $suboption and value is $suboption_data"
                    mta_v4_primary=`sysevent get MTA_DHCPv4_PrimaryAddress`
                    if [ "$mta_v4_primary" = "" ] ;then
                        mta_dhcp_option_received=1
                        echo "Setting MTA_DHCPv4_PrimaryAddress value as $suboption_data "
                        sysevent set MTA_DHCPv4_PrimaryAddress $suboption_data
                    fi
                ;;
                "02")
                    echo "Suboption is $suboption and value is $suboption_data"
                    mta_v4_secondary=`sysevent get MTA_DHCPv4_SecondaryAddress`
                    if [ "$mta_v4_secondary" = "" ] ;then
                        mta_dhcp_option_received=1
                        echo "Setting MTA_DHCPv4_SecondaryAddress value as $suboption_data "
                        sysevent set MTA_DHCPv4_SecondaryAddress $suboption_data
                    fi
                ;;
            esac
        done
    fi

    if [ -n "$opt125" ]; then
        echo "MTA DHCPv6 address received for OPT125 is $opt125"
        # Cut Enterprise ID and total length from option 125 received
        opt125=`echo $opt125 | awk '{print substr($0,CUR)}' CUR=11`
        echo "Opt125 is $opt125"
        parsed_value=""
        parsed_value=`parse_dhcp_option $opt125 v4`
        echo "OPT125 parsed value is $parsed_value"
        suboption=""
        suboption_data=""
        for val in $parsed_value; do
            suboption=`echo $val | cut -d = -f 1`
            suboption_data=`echo $val| cut -d = -f 2`
            case "$suboption" in
                "7c"|"7C")
                    echo "IP Pref received from dhcpv4 clients is $suboption_data"
                    Mta_Ip_Pref=`sysevent get MTA_IP_PREF`
                    if [ "$Mta_Ip_Pref" = "" ];then
                        echo "Setting MTA_IP_PREF value to $suboption_data"
                        sysevent set MTA_IP_PREF $suboption_data
                        mta_dhcp_option_received=1
                    else
                        echo "Mta_Ip_Pref value is already set to $Mta_Ip_Pref"
                    fi
                ;;
                "7b"|"7B")
                    echo "Suboption is $suboption and value is $suboption_data"
                    parsed_value_opt124=""
                    parsed_value_opt124=`parse_dhcp_option $suboption_data v4`
                    suboption_opt124=""
                    suboption_data_124=""
                    for val in $parsed_value_opt124; do
                        suboption_opt124=`echo $val | cut -d = -f 1`
                        suboption_data_124=`echo $val| cut -d = -f 2`
                        case "$suboption_opt124" in
                            "01")
                                echo "Suboption is $suboption_opt124 and value is $suboption_data_124"
                                mta_v6_primary=`sysevent get MTA_DHCPv6_PrimaryAddress`
                                if [ "$mta_v6_primary" = "" ] ;then
                                    echo "Setting MTA_DHCPv6_PrimaryAddress value as $suboption_data_124 "
                                    sysevent set MTA_DHCPv6_PrimaryAddress $suboption_data_124
                                    mta_dhcp_option_received=1
                                fi
                            ;;
                            "02")
                                echo "Suboption is $suboption_opt124 and value is $suboption_data_124"
                                mta_v6_secondary=`sysevent get MTA_DHCPv6_SecondaryAddress`
                                if [ "$mta_v6_secondary" = "" ] ;then
                                    echo "Setting MTA_DHCPv6_SecondaryAddress value as $suboption_data_124 "
                                    sysevent set MTA_DHCPv6_SecondaryAddress $suboption_data_124
                                    mta_dhcp_option_received=1
                                fi
                            ;;
                        esac
                    done
                ;;
            esac
        done
    fi

    if [ "$mta_dhcp_option_received" -eq 1 ];then
        echo "Setting dhcp_mta_option event as received"
        sysevent set dhcp_mta_option received
        mta_dhcp_option_received=0
    fi
fi
    if [ -n "$timezone" ]; then
        echo "$0: TimeOffset is $timezone"
        sysevent set ipv4-timeoffset @$timezone
    fi

    if [ -f /tmp/.ipv4dnsserver ]; then
        for i in $dns; do
            result=`grep $i /tmp/.ipv4dnsserver`
            if [ "$result" == "" ];then
                utc_time=`date -u`
                uptime=`cat /proc/uptime | awk '{ print $1 }' | cut -d"." -f1`
                echo "$utc_time DNS_server_IP_changed:$uptime" >> $CONSOLEFILE
                /usr/bin/onboarding_log "DNS_server_IP_changed:$uptime"
                echo $dns > /tmp/.ipv4dnsserver
            fi
        done
    else
        echo $dns > /tmp/.ipv4dnsserver
    fi

    # Update resolver configuration file
    R=""
    [ -n "$domain" ] && R="domain $domain
"
    for i in $dns; do
        echo "$0: Adding DNS $i"
        R="${R}nameserver $i
"
    done
    for i in $upstream_dns; do
        echo "nameserver $i"
    done > "$UPSTREAM_RESOLV_CONF"
    if [ "x$WAN_TYPE" == "xEPON" ]; then
        sysevent set wan_dns "$dns"
        sysevent set wan_domain $domain
        if [ -n  "$ntpsrv" ]; then
            oldntpsrv=`sysevent get wan_ntp_srv`
            if [ "$oldntpsrv" != "$ntpsrv" ]; then
                echo "$0: Update NTP List"
                sysevent set wan_ntp_srv "$ntpsrv"
                if [ -f /usr/ccsp/updateTimesyncdConf.sh ]; then
                    /usr/ccsp/updateTimesyncdConf.sh
                else
                    sed -i "/\<NTP\>/ s/$/ $ntpsrv/" "/etc/systemd/timesyncd.conf"
                fi
            fi
        fi
        if [ -n "$opt125" ]; then
            ipmode=${opt125:14}
            echo "$0: $ipmode"
            sysevent set wan4_ippref $ipmode
        fi
    fi

    if [ -x /sbin/resolvconf ]; then
        echo -n "$R" | /sbin/resolvconf -a "${interface}.udhcpc"
    else
        if [ "$over_ride_RESOLV_CONF" -eq 1 ]; then
            echo "Adding new IPV4 DNS SERVER to resolv.conf " >> $CONSOLEFILE
            echo -n "$R" >> $RESOLV_CONF_TMP
            N=""
            while read line; do
                N="${N}$line
"
            done < $RESOLV_CONF_TMP
            echo -n "$N" > "$RESOLV_CONF"
            echo "After renew change in IPV4 dns config so restarting dhcp-server(dnsmasq) " >> $CONSOLEFILE
            touch /var/tmp/.refreshlan
            sysevent set dhcp_server-restart
        else
            echo "NOT Adding new IPV4 DNS SERVER to resolv.conf " >> $CONSOLEFILE
        fi
        rm -rf $RESOLV_CONF_TMP
        sysevent set dhcp_domain $domain
    fi
}

echo "===$1==="

case "$1" in
        deconfig)
                echo "********apply deconfig********"
                if [ -x /sbin/resolvconf ]; then
                        /sbin/resolvconf -d "${interface}.udhcpc"
                fi
                if ! root_is_nfs ; then
                        if [ $have_bin_ip -eq 1 ]; then
                                ip -4 addr flush dev $interface
                                ip link set dev $interface up
                        else
                                /sbin/ifconfig $interface 0.0.0.0
                        fi
                fi
                ;;

        renew|bound)
            save_dhcp_offer $1
            NetworkConnectivityCheck=`sysevent get network_connectivity_check`
            NetworkConnectivityCheck=$((NetworkConnectivityCheck))
            if [ $NetworkConnectivityCheck -gt $((1)) ];then #Mask if NetworkConnectivityCheck > NET_CONNNECTIVITY_CHECK_ENABLED.
                NetworkConnectivityCheck=$(( NetworkConnectivityCheck&0xfe )) #Masking NET_CONNNECTIVITY_CHECK_ENABLED Value
            fi
            shift=$(( 4<<1 )) # here 4 is hardcode for NET_CONNNECTIVITY_CHECK_STARTED which is defined in service_wan.c
            echo "********NetworkConnectivityCheck=$NetworkConnectivityCheck, shift=$shift********"
            # there is two cases defined here, set of this values in sysevent is done in Wan Manager policy
            #1.) if NetworkConnectivityCheck is NET_CONNNECTIVITY_CHECK_DISABLED, which is 0 value.
            #2.) if NetworkConnectivityCheck is NET_CONNNECTIVITY_CHECK_STARTED, which is 4 value.
            if [ $NetworkConnectivityCheck -eq $((0)) ];then
                apply_configurations enable
            elif [ $NetworkConnectivityCheck -eq $shift ];then
                apply_configurations enable
            else
                echo "********interface=$interface, ip=$ip, mask=$mask, Broadcast=$BROADCAST, netmask=$NETMASK********"
                if [ $have_bin_ip -eq 1 ]; then
                    ip addr add dev $interface $ip/$mask $BROADCAST
                else
                    /sbin/ifconfig $interface $ip $BROADCAST $NETMASK
                fi
            fi
        ;;
        configure)
            echo "********apply configure********"
            apply_configurations
        ;;
        deconfigure)
            echo "********apply deconfigure********"
            apply_deconfigurations
        ;;
esac

exit 0

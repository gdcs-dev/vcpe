log-facility local7;

option space NEW_OPT_17 code width 2 length width 2;
option NEW_OPT_17.acs code 1 = text;
option NEW_OPT_17.provisioning code 2 = text;
option vsio.NEW_OPT_17 code 2636 = encapsulate NEW_OPT_17;

#----------------------------
#OPT17 namespace used for MV3 DUTs,
#opt17 should be encapsulated with 3561 for MV3 DUTs
option space MV3_OPT_17 code width 2 length width 2;
option MV3_OPT_17.acs code 1 = text;
option MV3_OPT_17.provisioning code 2 = text;
option vsio.MV3_OPT_17 code 3561 = encapsulate MV3_OPT_17;
#----------------------------

option NTP code 56 = string;

# ntp - ip
#option dhcp6.ntp-servers code 56 = {integer 16, integer 16, ip6-address} ;

# ntp - fqdn
option dhcp6.ntp-servers code 56 = {integer 16, integer 16, string} ;

# ntp array of ip
#option dhcp6.ntp-servers code 56 = array of ip6-address;


option dhcp6.sntp-servers 2a00:d78:0:712:94:198:159:14, 2a00:d78:0:712:94:198:159:10;



# erouter0
subnet6 2001:dbe:0:1::/64 {
    # Two addresses available to clients
    #  (the third client should get NoAddrsAvail)

    pool6 {
        #deny unknown-clients;
        range6 2001:dbe:0:1::130 2001:dbe:0:1::254;
    }

    # Use the whole /64 prefix for temporary addresses
    #  (i.e., direct application of RFC 4941)
    # range6 3ffe:501:ffff:100:: temporary;

    # Some /64 prefixes available for Prefix Delegation (RFC 3633)
    prefix6 3001:dbe:0:: 3001:dbe:0:f000:: /56;

    # no option 17 for erouter0
    # option dhcp6.vendor-opts 00:00:09:bf;
    # option NEW_OPT_17.acs "http://[2001:dbf:0:1::200]:9675";
    # option NEW_OPT_17.provisioning "TEXT";

    option dhcp6.name-servers 2001:dbe:0:1::129;

    # ntp server: server1b.meinberg.de  / alternative: time1.google.com
    option dhcp6.sntp-servers 2a01:4f8:a0:7143::2;

    # ntp1.revdomain.com
    # in hexadecimal ASCII is: 6e:74:70:31   2e   72:65:76:64:6f:6d:61:69:6e   63:6f:6d

    option dhcp6.ntp-servers 3 19 04:6e:74:70:32:09:72:65:76:64:6f:6d:61:69:6e:03:63:6f:6d;


    default-lease-time 3600; # 1000
    preferred-lifetime 3600; # 1000
    option dhcp-renewal-time 1800; # 60
    option dhcp-rebinding-time 2880; # 60
}

# mg0
subnet6 2001:dbd:0:1::/64 {
    # Two addresses available to clients
    #  (the third client should get NoAddrsAvail)

    pool6 {
        #deny unknown-clients;
        range6 2001:dbd:0:1::130 2001:dbd:0:1::254;
    }

    # Use the whole /64 prefix for temporary addresses
    #  (i.e., direct application of RFC 4941)
    # range6 3ffe:501:ffff:100:: temporary;

    # Some /64 prefixes available for Prefix Delegation (RFC 3633)

    # no prefix for management
    # prefix6 3001:dbd:0:: 3001:dbd:0:f000:: /56;

    # option dhcp6.vendor-opts 00:00:09:bf;

    # option 17 for mg0
    option MV3_OPT_17.acs "http://acs2.revdomain.com:9675";
    option MV3_OPT_17.provisioning "TEXT";


    option dhcp6.name-servers 2001:dbd:0:1::129;

    # ntp server: server1b.meinberg.de / alternative: time1.google.com
    #option dhcp6.sntp-servers 2a01:4f8:a0:7143::2;

    # ntp ip
    #option dhcp6.ntp-servers 1 16 2001:dbd:0:1::129;

    # ntp array of ip
    #option dhcp6.ntp-servers 2001:4860:4806::100, 2001:4860:4806::101, 2606:4700:f1::123, 2606:4700:f1::234;

    # ntp fqdn
    # 1st byte  3  is the the sub option for NTP FQDN
    # 2nd byte  11 is the length of the string
    # Rest of the sequence is the string quyntp.com coded in a particular way:
    # length - ascii code...- length -ascci code...
    # length is the length of the group of characters before a dot
    # 6 - quyntp in asccii - 3 com in asscii. 
    #
    # option dhcp6.ntp-servers 3 11 06:71:75:79:6e:74:70:03:63:6f:6d;
    #
    # ntp2.revdomain.com
    # in hexadecimal ASCII is: 6e:74:70:32   2e   72:65:76:64:6f:6d:61:69:6e   63:6f:6d

    option dhcp6.ntp-servers 3 19 04:6e:74:70:32:09:72:65:76:64:6f:6d:61:69:6e:03:63:6f:6d;


    default-lease-time 3600; # 1000
    preferred-lifetime 3600; # 1000
    option dhcp-renewal-time 1800; # 60
    option dhcp-rebinding-time 2880; # 60
}

# voip0
subnet6 2001:dbc:0:1::/64 {
    # Two addresses available to clients
    #  (the third client should get NoAddrsAvail)

    pool6 {
        #deny unknown-clients;
        range6 2001:dbc:0:1::130 2001:dbc:0:1::254;
    }

    # Use the whole /64 prefix for temporary addresses
    #  (i.e., direct application of RFC 4941)
    # range6 3ffe:501:ffff:100:: temporary;

    # Some /64 prefixes available for Prefix Delegation (RFC 3633)

    # no prefix for voice
    # prefix6 3001:dbd:0:: 3001:dbd:0:f000:: /56;

    option dhcp6.name-servers 2001:dbc:0:1::129;

    # ntp server: server1b.meinberg.de  / alternative: time1.google.com
    #option dhcp6.sntp-servers 2a01:4f8:a0:7143::2;

    #option dhcp6.ntp-servers 1 16 2001:dbd:0:1::129;
    #option dhcp6.ntp-servers 3 19 04:6e:74:70:32:09:72:65:76:64:6f:6d:61:69:6e:03:63:6f:6d;


    default-lease-time 3600; # 1000
    preferred-lifetime 3600; # 1000
    option dhcp-renewal-time 1800; # 60
    option dhcp-rebinding-time 2880; # 60
}


# wan single vlan erouter0 / no vlan
subnet6 2001:dae:0:1::/64 {
    # Two addresses available to clients
    #  (the third client should get NoAddrsAvail)

    pool6 {
        #deny unknown-clients;
        range6 2001:dae:0:1::130 2001:dae:0:1::254;
    }

    # Use the whole /64 prefix for temporary addresses
    #  (i.e., direct application of RFC 4941)
    # range6 3ffe:501:ffff:100:: temporary;

    # Some /64 prefixes available for Prefix Delegation (RFC 3633)
    prefix6 3001:dae:0:: 3001:dae:0:f000:: /56;

    # option 17 for single vlan erouter0
    # option dhcp6.vendor-opts 00:00:09:bf;
    ##option NEW_OPT_17.acs "http://acs1.revdomain.com:9675";
    ##option NEW_OPT_17.provisioning "TEXT";
    option MV3_OPT_17.acs "http://acs2.revdomain.com:9675";
    option MV3_OPT_17.provisioning "TEXT";


    option dhcp6.name-servers 2001:dae:0:1::129;

    # ntp server: server1b.meinberg.de  / alternative: time1.google.com
    option dhcp6.sntp-servers 2a01:4f8:a0:7143::2;

    # ntp ip
    #option dhcp6.ntp-servers 1 16 2001:dbd:0:1::129;

    # ntp array of ip
    #option dhcp6.ntp-servers 2001:4860:4806::100, 2001:4860:4806::101, 2606:4700:f1::123, 2606:4700:f1::234;

    # ntp fqdn
    # 1st byte  3  is the the sub option for NTP FQDN
    # 2nd byte  11 is the length of the string
    # Rest of the sequence is the string quyntp.com coded in a particular way:
    # length - ascii code...- length -ascci code...
    # length is the length of the group of characters before a dot
    # 6 - quyntp in asccii - 3 com in asscii. 
    #
    #option dhcp6.ntp-servers 3 11 06:71:75:79:6e:74:70:03:63:6f:6d;
    #
    # ntp1.revdomain.com
    # in hexadecimal ASCII is: 6e:74:70:31   2e   72:65:76:64:6f:6d:61:69:6e   63:6f:6d

    option dhcp6.ntp-servers 3 19 04:6e:74:70:31:09:72:65:76:64:6f:6d:61:69:6e:03:63:6f:6d;


    #### default-lease-time 1000;
    #### preferred-lifetime 1000;
    #### option dhcp-renewal-time 60;
    #### option dhcp-rebinding-time 60;

    default-lease-time 3600;
    preferred-lifetime 3600;
    option dhcp-renewal-time 1800;
    option dhcp-rebinding-time 2880;
}


# cm single vlan erouter0 / no vlan
subnet6 2001:daf:0:1::/64 {
    # Two addresses available to clients
    #  (the third client should get NoAddrsAvail)

    pool6 {
        #deny unknown-clients;
        range6 2001:daf:0:1::130 2001:daf:0:1::254;
    }

    # Use the whole /64 prefix for temporary addresses
    #  (i.e., direct application of RFC 4941)
    # range6 3ffe:501:ffff:100:: temporary;

    # Some /64 prefixes available for Prefix Delegation (RFC 3633)
    prefix6 3001:daf:0:: 3001:daf:0:f000:: /56;

    # option 17 for single vlan erouter0
    #option dhcp6.vendor-opts 00:00:09:bf;
    #option NEW_OPT_17.acs "http://acs1.revdomain.com:9675";
    #option NEW_OPT_17.provisioning "TEXT";
    option MV3_OPT_17.acs "http://acs2.revdomain.com:9675";
    option MV3_OPT_17.provisioning "TEXT";


    option dhcp6.name-servers 2001:daf:0:1::129;

    # ntp server: server1b.meinberg.de  / alternative: time1.google.com
    option dhcp6.sntp-servers 2a01:4f8:a0:7143::2;

    # ntp ip
    #option dhcp6.ntp-servers 1 16 2001:dbd:0:1::129;

    # ntp array of ip
    #option dhcp6.ntp-servers 2001:4860:4806::100, 2001:4860:4806::101, 2606:4700:f1::123, 2606:4700:f1::234;

    # ntp fqdn
    # 1st byte  3  is the the sub option for NTP FQDN
    # 2nd byte  11 is the length of the string
    # Rest of the sequence is the string quyntp.com coded in a particular way:
    # length - ascii code...- length -ascci code...
    # length is the length of the group of characters before a dot
    # 6 - quyntp in asccii - 3 com in asscii. 
    #
    #option dhcp6.ntp-servers 3 11 06:71:75:79:6e:74:70:03:63:6f:6d;
    #
    # ntp1.revdomain.com
    # in hexadecimal ASCII is: 6e:74:70:31   2e   72:65:76:64:6f:6d:61:69:6e   63:6f:6d

    option dhcp6.ntp-servers 3 19 04:6e:74:70:31:09:72:65:76:64:6f:6d:61:69:6e:03:63:6f:6d;


    #### default-lease-time 1000;
    #### preferred-lifetime 1000;
    #### option dhcp-renewal-time 60;
    #### option dhcp-rebinding-time 60;

    default-lease-time 3600;
    preferred-lifetime 3600;
    option dhcp-renewal-time 1800;
    option dhcp-rebinding-time 2880;
}

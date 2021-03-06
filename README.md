# ZerotierDNS

ztDNS is a dedicated DNS server for a ZeroTier virtual network. This project is a fork from the original one, which you can find [here on github](https://github.com/uxbh/ztdns)

## Overview

ztDNS pulls device names from Zerotier and makes them available by name using either IPv4 assigned addresses or IPv6 assigned addresses.

## Getting Started

### Traditional

If you prefer the traditional installation route:

#### Requirements

* [Go tools](https://golang.org/doc/install) - if not using a precompiled release

#### Install

1. Clone the repository and build it
    ``` bash
    git clone github.com/wendelb/ztdns
    go build
    ```
2. **If you are running on Linux**, run `sudo setcap cap_net_bind_service=+eip ./ztdns` to enable non-root users to bind privileged ports. On other operating systems, the program may need to be run as an administrator.

3. Add a new API access token to your user under the account tab at [https://my.zerotier.com](https://my.zerotier.com/).
    If you do not want to store your API access token in the configuration file you can also run the
    server with the `env` command: `env 'ZTDNS_ZT.API=<<APIToken>>' ./ztdns server`
4. Run `ztdns mkconfig` to generate a sample configuration file.
5. Add your API access token, Network names and IDs, and interface name to the configuration.
6. Start the server using `ztdns server`.
7. Add a DNS entry in your ZeroTier members pointing to the member running ztdns.

Once the server is up and running you will be able to resolve names based on the short name and suffix defined in the configuration file (zt by default) from ZeroTier.

```bash
dig @serveraddress member.domain.zt A
dig @serveraddress member.domain.zt AAAA
ping member.domain.zt
```

#### Running with systemd + apparmor

The recommended way on running this server is by running it confined via Apparmor and managed with Systemd. The provided configuration files restrict the application to the bare minimum it needs. It will also assign any necessary privileges and capabilities to run as a non-root user.

1. Clone the repo and build it as described in the Install section

2. Install it into `/opt/dnsserver`:
  * Create the directory if if does not already exist: `mkdir -p /opt/dnsserver`
  * Copy the executable into the folder `cp ztdns /opt/dnsserver/`
3. Create the configuration file
4. Register the AppArmor profile
  * Install the configuration `cp INSTALL/opt.dnsserver.ztdns /etc/apparmor.d/`
  * Make AppArmor aware or it `systemctl reload apparmor`
5. Install the systemd-unit
  * Copy to the target location `cp INSTALL/ztdns.service /etc/systemd/system/`

You can start the service using `systemctl start ztdns`. Once you are satisfied, autostart the service by issuing `systemctl enable ztdns`.

Congratulations, you now have ztdns up and running!

## Contributing

Thanks for considering contributing to the project. We welcome contributions, issues or requests from anyone, and are grateful for any help. Problems or questions? Feel free to open an issue on GitHub.

Please make sure your contributions adhere to the following guidelines:

* Code must adhere to the official Go [formating](https://golang.org/doc/effective_go.html#formatting) guidelines  (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
* Pull requests need to be based on and opened against the `master` branch.

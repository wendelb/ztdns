// Copyright © 2017 uxbh
// Copyright © 2019 Bernhard Wendel
// This file is part of github.com/wendelb/ztdns.

package cmd

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wendelb/ztdns/dnssrv"
	"github.com/wendelb/ztdns/ztapi"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run ztDNS server",
	Long: `Server (ztdns server) will start the DNS server.append
	
	Example: ztdns server`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Check if we are running as root and post a warning
		if (runtime.GOOS == "linux") || (runtime.GOOS == "darwin") {
			if os.Geteuid() == 0 {
				log.Warn("Running this application as root is discouraged. Please don't")
			}
		}

		// Check config and bail if anything important is missing.
		if viper.GetBool("debug") {
			log.SetLevel(log.DebugLevel)
			log.Debug("Setting Debug Mode")
		}
		if viper.GetString("ZT.API") == "" {
			return fmt.Errorf("no API key provided")
		}
		if len(viper.GetStringMapString("Networks")) == 0 {
			return fmt.Errorf("no Domain / Network ID pairs Provided")
		}
		if viper.GetString("ZT.URL") == "" {
			return fmt.Errorf("no URL provided. Run ztdns mkconfig first")
		}
		if viper.GetString("suffix") == "" {
			return fmt.Errorf("no DNS Suffix provided. Run ztdns mkconfig first")
		}
		if viper.GetString("myFQDN") == "" {
			return fmt.Errorf("no server name provided. Run ztdns mkconfig first")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Initialize all data structures that are needed for correct DNS Server execution
		initializeData()

		// Update the DNSDatabase
		lastUpdate := updateDNS()

		// Start the DNS server
		req := make(chan string)
		go dnssrv.Start(viper.GetString("interface"), viper.GetInt("port"), viper.GetString("suffix"), req, viper.GetString("myFQDN"))

		refresh := viper.GetInt("DbRefresh")
		if refresh == 0 {
			refresh = 30
		}
		for {
			// Block until a new request comes in
			n := <-req
			log.Debugf("Got request for %s", n)
			// If the database hasn't been updated in the last "refresh" minutes, update it.
			if time.Since(lastUpdate) > time.Duration(refresh)*time.Minute {
				log.Infof("DNSDatabase is stale. Refreshing.")
				lastUpdate = updateDNS()
			}
		}
	},
}

func init() {
	RootCmd.AddCommand(serverCmd)
	serverCmd.PersistentFlags().String("interface", "", "interface to listen on")
	viper.BindPFlag("interface", serverCmd.PersistentFlags().Lookup("interface"))
}

func initializeData() {
	suffix := viper.GetString("suffix")
	networks := viper.GetStringMapString("Networks")

	DNSDomains := make([]string, len(networks))
	i := 0
	for key := range networks {
		DNSDomains[i] = strings.ToLower(key + "." + suffix + ".")
		i++
	}

	dnssrv.DNSDomains = DNSDomains
}

func updateDNS() time.Time {
	// Get config info
	API := viper.GetString("ZT.API")
	URL := viper.GetString("ZT.URL")
	suffix := viper.GetString("suffix")

	foundDomains := make(map[string]bool)

	// Get all configured networks:
	for domain, id := range viper.GetStringMapString("Networks") {
		// Get ZeroTier Network info
		ztnetwork, err := ztapi.GetNetworkInfo(API, URL, id)
		if err != nil {
			log.Fatalf("Unable to update DNS entries: %s", err.Error())
		}

		// Get list of members in network
		log.Infof("Getting Members of Network: %s (%s)", ztnetwork.Config.Name, domain)
		lst, err := ztapi.GetMemberList(API, URL, ztnetwork.ID)
		if err != nil {
			log.Fatalf("Unable to update DNS entries: %s", err.Error())
		}
		log.Infof("Got %d members", len(*lst))

		for _, n := range *lst {
			// For all online members
			if n.Online {
				// Clear current DNS records
				record := strings.ToLower(n.Name + "." + domain + "." + suffix + ".")
				dnssrv.DNSDatabase[record] = dnssrv.Records{}
				ip6 := []net.IP{}
				ip4 := []net.IP{}
				// Get 6Plane address if network has it enabled
				if ztnetwork.Config.V6AssignMode.Sixplane {
					ip6 = append(ip6, n.Get6Plane())
				}
				// Get RFC4193 address if network has it enabled
				if ztnetwork.Config.V6AssignMode.Rfc4193 {
					ip6 = append(ip6, n.GetRFC4193())
				}

				// Get the rest of the address assigned to the member
				for _, a := range n.Config.IPAssignments {
					ip4 = append(ip4, net.ParseIP(a))
				}
				// Add the record to the database
				log.Infof("Updating %-15s IPv4: %-15s IPv6: %s", record, ip4, ip6)
				dnssrv.DNSDatabase[record] = dnssrv.Records{
					A:    ip4,
					AAAA: ip6,
				}
				foundDomains[record] = true
			}
		}
	}

	// Now clean all domains drom the DNSDatabase that were not in the update from the API
	for record := range dnssrv.DNSDatabase {
		_, isValid := foundDomains[record]
		if !isValid {
			log.Infof("Removing stale domain %s", record)
			delete(dnssrv.DNSDatabase, record)
		}
	}

	// Return the current update time
	return time.Now()
}

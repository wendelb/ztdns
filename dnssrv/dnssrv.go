// Copyright © 2017 uxbh
// Copyright © 2019 Bernhard Wendel
// This file is part of github.com/wendelb/ztdns.

// Package dnssrv implements a simple DNS server.
package dnssrv

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)

// Records contains the types of records the server will respond to.
type Records struct {
	A    []net.IP
	AAAA []net.IP
}

// DNSUpdate is the last time the DNSDatabase was updated.
var DNSUpdate = time.Time{}

// DNSDatabase is a map of hostnames to the records associated with it.
var DNSDatabase = map[string]Records{}

var DNSDomains []string

var queryChan chan string

var soaTemplate string
var nsTemplate string
var myIP dns.RR

// Start brings up a DNS server for the specified suffix on a given port.
func Start(iface string, port int, suffix string, req chan string, myFQDN string) error {
	queryChan = req

	if port == 0 {
		port = 53
	}

	// attach request handler func
	dns.HandleFunc(suffix+".", handleDNSRequest)

	// Get my IP for correct NS response
	serverIP := getIPFromDNS(myFQDN)
	myIP, _ = dns.NewRR(fmt.Sprintf("%s A %s", myFQDN, serverIP))

	// Create SOA + NS Record for reuse
	soaTemplate = fmt.Sprintf("%%s 3600 IN SOA %s postmaster.%s %d 14400 3600 604800 60", myFQDN, myFQDN, time.Now().Unix())
	nsTemplate = fmt.Sprintf("%%s 3600 IN NS %s", myFQDN)

	for _, addr := range getIfaceAddrs(iface) {
		go func(suffix string, addr net.IP, port int) {
			var server *dns.Server
			if addr.To4().String() == addr.String() {
				log.Debugf("Creating IPv4 Server: %s:%d udp", addr, port)
				server = &dns.Server{
					Addr: fmt.Sprintf("%s:%d", addr, port),
					Net:  "udp",
				}
			} else {
				log.Debugf("Creating IPv6 Server: [%s]:%d udp6", addr, port)
				server = &dns.Server{
					Addr: fmt.Sprintf("[%s]:%d", addr, port),
					Net:  "udp6",
				}
			}
			log.Printf("Starting server for %s on %s", suffix, server.Addr)
			err := server.ListenAndServe()
			if err != nil {
				log.Fatalf("failed to start DNS server: %s", err.Error())
			}
			defer server.Shutdown()
		}(suffix, addr, port)
	}
	return nil
}

func getIPFromDNS(domain string) string {
	lookupResult, err := net.LookupIP(domain)

	if err != nil {
		log.Fatalf("Cannot lookup name %s: %s", domain, err.Error())
	}

	return lookupResult[0].String()
}

func getIfaceAddrs(iface string) []net.IP {
	if iface != "" {
		retaddrs := []net.IP{}
		netint, err := net.InterfaceByName(iface)
		if err != nil {
			log.Fatalf("Could not get interface: %s\n", err.Error())
		}
		addrs, err := netint.Addrs()
		if err != nil {
			log.Fatalf("Could not get addresses: %s\n", err.Error())
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			if !ip.IsLinkLocalUnicast() {
				log.Debugf("Found address: %s", ip.String())
				retaddrs = append(retaddrs, ip)
			}
		}
		return retaddrs
	}
	return []net.IP{net.IPv4zero}
}

func findSuffix(domain string) string {
	for _, v := range DNSDomains {
		if strings.HasSuffix(domain, v) {
			return v
		}
	}

	return ""
}

// handleDNSRequest routes an incoming DNS request to a parser.
func handleDNSRequest(w dns.ResponseWriter, request *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(request)
	m.Compress = false
	m.Authoritative = true

	if request.Opcode == dns.OpcodeQuery {
		for _, q := range m.Question {
			queryChan <- q.Name
			lookupName := strings.ToLower(q.Name)

			// Find Domain from Name
			domainName := findSuffix(lookupName)

			if domainName == "" {
				// The requested domain is not handled by this server
				m.SetRcode(request, dns.RcodeRefused)
				continue
			}

			// handle ANY queries
			if q.Qtype == dns.TypeANY {
				rr, err := dns.NewRR(fmt.Sprintf("%s HINFO \"RFC8482\" \"\"", q.Name))
				if err == nil {
					m.Answer = append(m.Answer, rr)
				}
				continue
			}

			if (q.Qtype == dns.TypeSOA) && (domainName == lookupName) {
				rrSOA, err1 := dns.NewRR(fmt.Sprintf(soaTemplate, domainName))
				rrNS, err2 := dns.NewRR(fmt.Sprintf(nsTemplate, domainName))
				if (err1 == nil) && (err2 == nil) {
					m.Answer = append(m.Answer, rrSOA)
					m.Ns = append(m.Ns, rrNS)
					m.Extra = append(m.Extra, dns.Copy(myIP))
				}
				continue
			}

			if (q.Qtype == dns.TypeNS) && (domainName == lookupName) {
				rr, err := dns.NewRR(fmt.Sprintf(nsTemplate, domainName))
				if err == nil {
					m.Answer = append(m.Answer, rr)
					m.Extra = append(m.Extra, dns.Copy(myIP))
				}
				continue
			}

			if rec, ok := DNSDatabase[lookupName]; ok {
				// We have someone with this name
				switch q.Qtype {
				case dns.TypeA:
					for _, ip := range rec.A {
						rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, ip.String()))
						if err == nil {
							m.Answer = append(m.Answer, rr)
						}
					}
				case dns.TypeAAAA:
					for _, ip := range rec.AAAA {
						rr, err := dns.NewRR(fmt.Sprintf("%s AAAA %s", q.Name, ip.String()))
						if err == nil {
							m.Answer = append(m.Answer, rr)
						}
					}
				}

				if len(m.Answer) == 0 {
					soa, _ := dns.NewRR(fmt.Sprintf(soaTemplate, domainName))
					m.Ns = append(m.Ns, soa)
				}
			} else {
				// We don't have someone with this name -> return record not found
				m.SetRcode(request, dns.RcodeNameError)
				soa, _ := dns.NewRR(fmt.Sprintf(soaTemplate, domainName))
				m.Ns = append(m.Ns, soa)
			}
		}

	}

	w.WriteMsg(m)
}

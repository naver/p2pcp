// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package testutil

import (
	"encoding/binary"
	"log"
	"net"
	"strings"
	"sync"
)

// SRVRecord represents an SRV record
type SRVRecord struct {
	Priority uint16
	Weight   uint16
	Port     uint16
	Target   string
}

type DNSMockServer struct {
	addr         string
	records      map[string][]string    // Domain -> IP list mapping (for A records)
	srvRecords   map[string][]SRVRecord // Domain -> SRV record list mapping
	conn         *net.UDPConn
	mu           sync.RWMutex
	requestCount int
}

func NewDNSMockServer(addr string) *DNSMockServer {
	return &DNSMockServer{
		addr:       addr,
		records:    make(map[string][]string),
		srvRecords: make(map[string][]SRVRecord),
	}
}

func (s *DNSMockServer) AddRecord(domain string, ips []string) {
	// Normalize domain to lowercase and append trailing dot
	domain = strings.ToLower(domain)
	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}
	s.mu.Lock()
	s.records[domain] = ips
	s.mu.Unlock()
}

// AddDynamicRecord adds a record that changes based on request count
func (s *DNSMockServer) AddDynamicRecord(domain string, allIPs []string) {
	domain = strings.ToLower(domain)
	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}
	s.mu.Lock()
	s.records[domain] = allIPs // Store entire IP list
	s.mu.Unlock()
}

// AddSRVRecord adds an SRV record
func (s *DNSMockServer) AddSRVRecord(domain string, srvRecords []SRVRecord) {
	domain = strings.ToLower(domain)
	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}
	s.mu.Lock()
	s.srvRecords[domain] = srvRecords
	s.mu.Unlock()
}

// AddDynamicSRVRecord adds SRV records that change based on request count
func (s *DNSMockServer) AddDynamicSRVRecord(domain string, allSrvRecords []SRVRecord) {
	domain = strings.ToLower(domain)
	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}
	s.mu.Lock()
	s.srvRecords[domain] = allSrvRecords
	s.mu.Unlock()
}

func (s *DNSMockServer) Start() error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return err
	}

	s.conn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	log.Printf("Starting DNS mock server: %s", s.addr)

	buf := make([]byte, 512)
	for {
		n, clientAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		response := s.handleQuery(buf[:n])
		if response != nil {
			_, err = s.conn.WriteToUDP(response, clientAddr)
			if err != nil {
				log.Printf("Write error: %v", err)
			}
		}
	}
}

func (s *DNSMockServer) Stop() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func (s *DNSMockServer) handleQuery(query []byte) []byte {
	if len(query) < 12 {
		return nil
	}

	// Parse DNS header
	transactionID := query[0:2]

	// Extract domain name (after 12-byte header)
	domain, offset := parseDomainName(query, 12)
	domain = strings.ToLower(domain)

	// Check QTYPE and QCLASS
	if offset+4 > len(query) {
		return nil
	}
	qtype := binary.BigEndian.Uint16(query[offset : offset+2])

	s.mu.Lock()
	defer s.mu.Unlock()

	if qtype == 1 { // TYPE A
		allIPs, exists := s.records[domain]
		if !exists {
			return s.buildNXDomainResponse(transactionID, query[12:offset+4])
		}

		// Dynamic change logic
		s.requestCount++
		count := s.requestCount

		var selectedIPs []string
		if len(allIPs) > 3 {
			// Dynamic change: first 5 requests return 3 IPs, middle 10 return all, rest return 3
			if count <= 5 {
				selectedIPs = allIPs[:3]
			} else if count <= 15 {
				selectedIPs = allIPs
			} else {
				selectedIPs = allIPs[:3]
			}
		} else {
			selectedIPs = allIPs
		}

		return s.buildMultiResponse(transactionID, query[12:offset+4], selectedIPs)

	} else if qtype == 28 { // TYPE AAAA (IPv6)
		// IPv6 not supported, return NXDOMAIN response
		return s.buildNXDomainResponse(transactionID, query[12:offset+4])

	} else if qtype == 33 { // TYPE SRV
		allSrvRecords, exists := s.srvRecords[domain]
		if !exists {
			return s.buildNXDomainResponse(transactionID, query[12:offset+4])
		}

		// Dynamic change logic
		s.requestCount++
		count := s.requestCount

		var selectedSrvRecords []SRVRecord
		if len(allSrvRecords) > 3 {
			// Dynamic change: first 5 requests return 3 records, middle 10 return all, rest return 3
			if count <= 5 {
				selectedSrvRecords = allSrvRecords[:3]
			} else if count <= 15 {
				selectedSrvRecords = allSrvRecords
			} else {
				selectedSrvRecords = allSrvRecords[:3]
			}
		} else {
			selectedSrvRecords = allSrvRecords
		}

		return s.buildSRVResponse(transactionID, query[12:offset+4], selectedSrvRecords)
	}

	return nil
}

func parseDomainName(data []byte, offset int) (string, int) {
	var domain []string
	pos := offset

	for pos < len(data) {
		length := int(data[pos])
		if length == 0 {
			pos++
			break
		}
		pos++
		if pos+length > len(data) {
			break
		}
		domain = append(domain, string(data[pos:pos+length]))
		pos += length
	}

	return strings.Join(domain, ".") + ".", pos
}

func (s *DNSMockServer) buildMultiResponse(transactionID, question []byte, ips []string) []byte {
	response := make([]byte, 0, 512)

	// Transaction ID
	response = append(response, transactionID...)

	// Flags: standard query response, recursion available
	response = append(response, 0x81, 0x80)

	// Questions: 1
	response = append(response, 0x00, 0x01)

	// Answer RRs: len(ips)
	response = append(response, 0x00, byte(len(ips)))

	// Authority RRs: 0
	response = append(response, 0x00, 0x00)

	// Additional RRs: 0
	response = append(response, 0x00, 0x00)

	// Copy Question section
	response = append(response, question...)

	// Answer section - for each IP
	for _, ip := range ips {
		// Name (use pointer to reference domain in question)
		response = append(response, 0xc0, 0x0c)

		// Type: A (1)
		response = append(response, 0x00, 0x01)

		// Class: IN (1)
		response = append(response, 0x00, 0x01)

		// TTL: 300 seconds
		response = append(response, 0x00, 0x00, 0x01, 0x2c)

		// Data length: 4
		response = append(response, 0x00, 0x04)

		// IP address
		ipBytes := net.ParseIP(ip).To4()
		response = append(response, ipBytes...)
	}

	return response
}

func (s *DNSMockServer) buildResponse(transactionID, question []byte, ip string) []byte {
	return s.buildMultiResponse(transactionID, question, []string{ip})
}

func (s *DNSMockServer) buildNXDomainResponse(transactionID, question []byte) []byte {
	response := make([]byte, 0, 512)

	// Transaction ID
	response = append(response, transactionID...)

	// Flags: NXDOMAIN (name not found)
	response = append(response, 0x81, 0x83)

	// Questions: 1
	response = append(response, 0x00, 0x01)

	// Answer RRs: 0
	response = append(response, 0x00, 0x00)

	// Authority RRs: 0
	response = append(response, 0x00, 0x00)

	// Additional RRs: 0
	response = append(response, 0x00, 0x00)

	// Copy Question section
	response = append(response, question...)

	return response
}

func (s *DNSMockServer) buildSRVResponse(transactionID, question []byte, srvRecords []SRVRecord) []byte {
	response := make([]byte, 0, 512)

	// Transaction ID
	response = append(response, transactionID...)

	// Flags: standard query response, recursion available
	response = append(response, 0x81, 0x80)

	// Questions: 1
	response = append(response, 0x00, 0x01)

	// Answer RRs: len(srvRecords)
	response = append(response, 0x00, byte(len(srvRecords)))

	// Authority RRs: 0
	response = append(response, 0x00, 0x00)

	// Additional RRs: 0
	response = append(response, 0x00, 0x00)

	// Copy Question section
	response = append(response, question...)

	// Answer section - for each SRV record
	for _, srv := range srvRecords {
		// Name (use pointer to reference domain in question)
		response = append(response, 0xc0, 0x0c)

		// Type: SRV (33)
		response = append(response, 0x00, 0x21)

		// Class: IN (1)
		response = append(response, 0x00, 0x01)

		// TTL: 300 seconds
		response = append(response, 0x00, 0x00, 0x01, 0x2c)

		// Calculate data length (Priority + Weight + Port + Target length)
		targetBytes := encodeDomainName(srv.Target)
		dataLength := 6 + len(targetBytes) // 2+2+2 + target length
		response = append(response, byte(dataLength>>8), byte(dataLength))

		// Priority
		response = append(response, byte(srv.Priority>>8), byte(srv.Priority))

		// Weight
		response = append(response, byte(srv.Weight>>8), byte(srv.Weight))

		// Port
		response = append(response, byte(srv.Port>>8), byte(srv.Port))

		// Target
		response = append(response, targetBytes...)
	}

	return response
}

// encodeDomainName encodes a domain name in DNS format
func encodeDomainName(domain string) []byte {
	// Convert IP address to FQDN if applicable
	if net.ParseIP(domain) != nil {
		// Convert IP address to hostname instead of using as-is
		domain = domain + ".local"
	}

	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}

	parts := strings.Split(domain, ".")
	var result []byte

	for _, part := range parts {
		if part == "" {
			break // Last empty part
		}
		if len(part) > 63 { // DNS label length limit
			part = part[:63]
		}
		result = append(result, byte(len(part)))
		result = append(result, []byte(part)...)
	}
	result = append(result, 0) // Mark end of domain

	return result
}

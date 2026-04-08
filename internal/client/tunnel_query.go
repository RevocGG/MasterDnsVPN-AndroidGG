// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================
// Package client provides the core logic for the MasterDnsVPN client.
// This file (tunnel_query.go) handles the construction of DNS tunnel queries.
// ==============================================================================
package client

import (
	DnsParser "masterdnsvpn-go/internal/dnsparser"
	Enums "masterdnsvpn-go/internal/enums"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

type preparedTunnelDomain struct {
	normalized string
	qname      []byte
}

func buildTunnelTXTQuestionBytes(domain string, encoded []byte) ([]byte, error) {
	return DnsParser.BuildTunnelTXTQuestionPacket(domain, encoded, Enums.DNS_RECORD_TYPE_TXT, EDnsSafeUDPSize)
}

func prepareTunnelDomain(domain string) (preparedTunnelDomain, error) {
	normalized, qname, err := DnsParser.PrepareTunnelDomainQname(domain)
	if err != nil {
		return preparedTunnelDomain{}, err
	}
	return preparedTunnelDomain{normalized: normalized, qname: qname}, nil
}

// buildTunnelTXTQueryRaw builds an encoded tunnel query using the provided options and codec.
func (c *Client) buildTunnelTXTQueryRaw(domain string, options VpnProto.BuildOptions) ([]byte, error) {
	raw, err := VpnProto.BuildRaw(options)
	if err != nil {
		return nil, err
	}
	encoded, err := c.codec.EncryptAndEncodeBytes(raw)
	if err != nil {
		return nil, err
	}
	return buildTunnelTXTQuestionBytes(domain, encoded)
}

func (c *Client) buildEncodedAutoWithCompressionTrace(options VpnProto.BuildOptions) ([]byte, error) {
	raw, err := VpnProto.BuildRawAuto(options, c.cfg.CompressionMinSize)
	if err != nil {
		return nil, err
	}

	if c.codec == nil {
		return nil, VpnProto.ErrCodecUnavailable
	}
	return c.codec.EncryptAndEncodeBytes(raw)
}

// buildTunnelTXTQuery builds an encoded tunnel query with automatic option handling.
func (c *Client) buildTunnelTXTQuery(domain string, options VpnProto.BuildOptions) ([]byte, error) {
	encoded, err := c.buildEncodedAutoWithCompressionTrace(options)
	if err != nil {
		return nil, err
	}
	return buildTunnelTXTQuestionBytes(domain, encoded)
}

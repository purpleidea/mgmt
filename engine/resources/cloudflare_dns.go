// Mgmt
// Copyright (C) James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

package resources

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/dns"
	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/cloudflare/cloudflare-go/v6/zones"
)

func init() {
	engine.RegisterResource("cloudflare:dns", func() engine.Res { return &CloudflareDNSRes{} })
}

// CloudflareDNSRes is a resource for managing DNS records in Cloudflare zones.
// This resource uses the Cloudflare API to create, update, and delete DNS
// records in a specified zone. It supports various record types including A,
// AAAA, CNAME, MX, TXT, NS, and PTR records. The resource requires polling to
// detect changes, as the Cloudflare API does not provide an event stream. The
// Purge functionality allows enforcing that only managed DNS records exist in
// the zone, removing any unmanaged records.
type CloudflareDNSRes struct {
	traits.Base
	traits.GraphQueryable
	init *engine.Init

	// APIToken is the Cloudflare API token used for authentication. This is
	// required and must have the necessary permissions to manage DNS records
	// in the specified zone.
	APIToken string `lang:"apitoken"`

	// Comment is an optional comment to attach to the DNS record. This is
	// stored in Cloudflare and can be used for documentation purposes.
	Comment string `lang:"comment"`

	// Content is the value for the DNS record. This is required when State
	// is "exists" unless Purge is true. The format depends on the record
	// Type (e.g., IP address for A records, hostname for CNAME records).
	Content string `lang:"content"`

	// Priority is the priority value for records that support it (e.g., MX
	// records). This is a pointer to distinguish between an unset value and
	// a zero value.
	Priority *int64 `lang:"priority"`

	// Proxied specifies whether the record should be proxied through
	// Cloudflare's CDN. This is a pointer to distinguish between an unset
	// value and false. Only applicable to certain record types.
	Proxied *bool `lang:"proxied"`

	// Purge specifies whether to delete all DNS records in the zone that are
	// not defined in the mgmt graph. When true, this resource will query the
	// graph for other cloudflare:dns resources in the same zone and delete
	// any records not managed by those resources.
	Purge bool `lang:"purge"`

	// RecordName is the name of the DNS record (e.g., "www.example.com" or
	// "@" for the zone apex). This is required.
	RecordName string `lang:"record_name"`

	// State determines whether the DNS record should exist or be absent.
	// Valid values are "exists" (default) or "absent". When set to "absent",
	// the record will be deleted if it exists.
	State string `lang:"state"`

	// TTL is the time-to-live value for the DNS record in seconds. Must be
	// between 60 and 86400, or set to 1 for automatic TTL. Default is 1.
	TTL int64 `lang:"ttl"`

	// Type is the DNS record type (e.g., "A", "AAAA", "CNAME", "MX", "TXT",
	// "NS", "SRV", "PTR"). This is required.
	Type string `lang:"type"`

	// Zone is the name of the Cloudflare zone (domain) where the DNS record
	// should be managed (e.g., "example.com"). This is required.
	Zone string `lang:"zone"`

	client *cloudflare.Client
	zoneID string
}

// Default returns some sensible defaults for this resource.
func (obj *CloudflareDNSRes) Default() engine.Res {
	return &CloudflareDNSRes{
		State: "exists",
		TTL:   1, // this sets TTL to automatic
	}
}

// Validate checks if the resource data structure was populated correctly.
func (obj *CloudflareDNSRes) Validate() error {
	if obj.RecordName == "" {
		return fmt.Errorf("record name is required")
	}

	if obj.APIToken == "" {
		return fmt.Errorf("api token is required")
	}

	if obj.Type == "" {
		return fmt.Errorf("record type is required")
	}

	if (obj.TTL < 60 || obj.TTL > 86400) && obj.TTL != 1 { // API requirement
		return fmt.Errorf("ttl must be between 60s and 86400s, or set to 1")
	}

	if obj.Zone == "" {
		return fmt.Errorf("zone name is required")
	}

	if obj.State != "exists" && obj.State != "absent" && obj.State != "" {
		return fmt.Errorf("state must be either 'exists', 'absent', or empty")
	}

	if obj.State == "exists" && obj.Content == "" && !obj.Purge {
		return fmt.Errorf("content is required when state is 'exists'")
	}

	if obj.Type == "MX" && obj.Priority == nil {
		return fmt.Errorf("priority is required for MX records")
	}

	if obj.MetaParams().Poll == 0 || obj.MetaParams().Poll < 60 {
		return fmt.Errorf("cloudflare:dns requires polling, set Meta:poll param (e.g., 300s), min. 60s")
	}

	return nil
}

// Init runs some startup code for this resource. It initializes the Cloudflare
// API client and validates that the specified zone exists.
func (obj *CloudflareDNSRes) Init(init *engine.Init) error {
	obj.init = init

	obj.client = cloudflare.NewClient(
		option.WithAPIToken(obj.APIToken),
	)

	zoneListParams := zones.ZoneListParams{
		Name: cloudflare.F(obj.Zone),
	}

	zoneList, err := obj.client.Zones.List(context.Background(), zoneListParams)
	if err != nil {
		return errwrap.Wrapf(err, "failed to list zones")
	}

	if len(zoneList.Result) == 0 {
		return fmt.Errorf("zone %s not found", obj.Zone)
	}

	obj.zoneID = zoneList.Result[0].ID

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done. It
// clears sensitive data and releases the API client connection.
func (obj *CloudflareDNSRes) Cleanup() error {
	obj.APIToken = ""
	obj.client = nil
	obj.zoneID = ""
	return nil
}

// Watch isn't implemented for this resource, since the Cloudflare API does not
// provide any event stream. Instead, always use polling.
func (obj *CloudflareDNSRes) Watch(context.Context) error {
	return fmt.Errorf("invalid Watch call: requires poll metaparam")
}

// CheckApply is the main convergence function for this resource. It checks the
// current state of the DNS record against the desired state and applies changes
// if necessary. If apply is false, it only checks if changes are needed. If
// Purge is enabled, it will first delete any unmanaged records in the zone.
func (obj *CloudflareDNSRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	// we start by checking the need for purging
	if obj.Purge {
		checkOK, err := obj.purgeCheckApply(ctx, apply)
		if err != nil {
			return false, err
		}
		if !checkOK {
			return false, nil
		}
	}

	// use DNS lookup when not deleting records
	if obj.State != "absent" {
		exists, matches, err := obj.lookupRecordViaDNS(ctx)
		if err != nil {
			obj.init.Logf("dns lookup failed, falling back to API: %v", err)
		} else {
			switch {
			case !exists && obj.State == "exists":
				if !apply {
					return false, nil
				}
				if err := obj.createRecord(ctx); err != nil {
					return false, err
				}
				return true, nil
			case exists && matches && !obj.needsMetadataUpdate():
				obj.init.Logf("DNS content matches and no need to query API")
				return true, nil
			case exists && matches && obj.needsMetadataUpdate():
				obj.init.Logf("DNS content matches, verifying metadata via API")
			case exists && !matches:
				obj.init.Logf("DNS content mismatch, will update via API")
			}
		}
	}

	// we're using `contains` so as to get the candidates, as `exact` might not
	// give the expected results depending on how the user specified it
	listParams := dns.RecordListParams{
		ZoneID: cloudflare.F(obj.zoneID),
		Name: cloudflare.F(dns.RecordListParamsName{
			Contains: cloudflare.F(obj.RecordName),
		}),
		Type: cloudflare.F(dns.RecordListParamsType(obj.Type)),
	}

	recordList, err := obj.client.DNS.Records.List(ctx, listParams)
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to list DNS records")
	}

	// here we filter to find the exact match
	recordExists := false
	var record dns.RecordResponse
	for _, r := range recordList.Result {
		if obj.matchesRecordName(r.Name) {
			record = r
			recordExists = true
			break
		}
	}

	switch obj.State {
	case "exists", "":
		if !recordExists {
			if !apply {
				return false, nil
			}

			if err := obj.createRecord(ctx); err != nil {
				return false, err
			}

			obj.init.Logf("created DNS record: %s %s -> %s", obj.Type, obj.RecordName, obj.Content)
			return true, nil
		}

		if obj.needsUpdate(record) {
			if !apply {
				return false, nil
			}

			if err := obj.updateRecord(ctx, record.ID); err != nil {
				return false, err
			}

			obj.init.Logf("updated DNS record: %s %s -> %s", obj.Type, obj.RecordName, obj.Content)
			return true, nil
		}

	case "absent":
		if recordExists {
			if !apply {
				return false, nil
			}

			deleteParams := dns.RecordDeleteParams{
				ZoneID: cloudflare.F(obj.zoneID),
			}

			_, err := obj.client.DNS.Records.Delete(ctx, record.ID, deleteParams)
			if err != nil {
				return false, errwrap.Wrapf(err, "failed to delete DNS record")
			}

			obj.init.Logf("deleted DNS record: %s %s", obj.Type, obj.RecordName)
			return true, nil
		}
	}

	return true, nil
}

// Cmp compares two resources and returns an error if they differ. This is used
// to determine if two resources are equivalent for graph operations.
func (obj *CloudflareDNSRes) Cmp(r engine.Res) error {
	if obj == nil && r == nil {
		return nil
	}

	if (obj == nil) != (r == nil) {
		return fmt.Errorf("one resource is empty")
	}

	res, ok := r.(*CloudflareDNSRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.APIToken != res.APIToken {
		return fmt.Errorf("apitoken differs")
	}

	if (obj.Proxied == nil) != (res.Proxied == nil) {
		return fmt.Errorf("proxied values differ")
	}

	if obj.Proxied != nil && *obj.Proxied != *res.Proxied {
		return fmt.Errorf("proxied values differ")
	}

	if obj.RecordName != res.RecordName {
		return fmt.Errorf("record name differs")
	}

	if obj.Purge != res.Purge {
		return fmt.Errorf("purge value differs")
	}

	if obj.State != res.State {
		return fmt.Errorf("state differs")
	}

	if obj.TTL != res.TTL {
		return fmt.Errorf("ttl differs")
	}

	if obj.Type != res.Type {
		return fmt.Errorf("record type differs")
	}

	if obj.Zone != res.Zone {
		return fmt.Errorf("zone differs")
	}

	if obj.zoneID != res.zoneID {
		return fmt.Errorf("zoneid differs")
	}

	if obj.Content != res.Content {
		return fmt.Errorf("content param differs")
	}

	if (obj.Priority == nil) != (res.Priority == nil) {
		return fmt.Errorf("the priority param differs")
	}

	if obj.Priority != nil && *obj.Priority != *res.Priority {
		return fmt.Errorf("the priority param differs")
	}

	return nil
}

// buildRecordParam creates the appropriate record parameter structure based on
// the record type. This is a helper function used by buildNewRecordParam and
// buildEditRecordParam.
// TODO: double check the fields for each record, might have missed some
func (obj *CloudflareDNSRes) buildRecordParam() (any, error) {
	ttl := dns.TTL(obj.TTL)

	switch obj.Type {
	case "A":
		param := dns.ARecordParam{
			Name:    cloudflare.F(obj.RecordName),
			Type:    cloudflare.F(dns.ARecordTypeA),
			Content: cloudflare.F(obj.Content),
			TTL:     cloudflare.F(ttl),
		}
		if obj.Proxied != nil {
			param.Proxied = cloudflare.F(*obj.Proxied)
		}
		if obj.Comment != "" {
			param.Comment = cloudflare.F(obj.Comment)
		}
		return param, nil

	case "AAAA":
		param := dns.AAAARecordParam{
			Name:    cloudflare.F(obj.RecordName),
			Type:    cloudflare.F(dns.AAAARecordTypeAAAA),
			Content: cloudflare.F(obj.Content),
			TTL:     cloudflare.F(ttl),
		}
		if obj.Proxied != nil {
			param.Proxied = cloudflare.F(*obj.Proxied)
		}
		if obj.Comment != "" {
			param.Comment = cloudflare.F(obj.Comment)
		}
		return param, nil

	case "CNAME":
		param := dns.CNAMERecordParam{
			Name:    cloudflare.F(obj.RecordName),
			Type:    cloudflare.F(dns.CNAMERecordTypeCNAME),
			Content: cloudflare.F(obj.Content),
			TTL:     cloudflare.F(ttl),
		}
		if obj.Proxied != nil {
			param.Proxied = cloudflare.F(*obj.Proxied)
		}
		if obj.Comment != "" {
			param.Comment = cloudflare.F(obj.Comment)
		}
		return param, nil

	case "MX":
		param := dns.MXRecordParam{
			Name:    cloudflare.F(obj.RecordName),
			Type:    cloudflare.F(dns.MXRecordTypeMX),
			Content: cloudflare.F(obj.Content),
			TTL:     cloudflare.F(ttl),
		}
		if obj.Proxied != nil {
			param.Proxied = cloudflare.F(*obj.Proxied)
		}
		if obj.Priority != nil { // required for MX record
			param.Priority = cloudflare.F(float64(*obj.Priority))
		}
		if obj.Comment != "" {
			param.Comment = cloudflare.F(obj.Comment)
		}
		return param, nil

	case "TXT":
		param := dns.TXTRecordParam{
			Name:    cloudflare.F(obj.RecordName),
			Type:    cloudflare.F(dns.TXTRecordTypeTXT),
			Content: cloudflare.F(obj.Content),
			TTL:     cloudflare.F(ttl),
		}
		if obj.Proxied != nil {
			param.Proxied = cloudflare.F(*obj.Proxied)
		}
		if obj.Comment != "" {
			param.Comment = cloudflare.F(obj.Comment)
		}
		return param, nil

	case "NS":
		param := dns.NSRecordParam{
			Name:    cloudflare.F(obj.RecordName),
			Type:    cloudflare.F(dns.NSRecordTypeNS),
			Content: cloudflare.F(obj.Content),
			TTL:     cloudflare.F(ttl),
		}
		if obj.Proxied != nil {
			param.Proxied = cloudflare.F(*obj.Proxied)
		}
		if obj.Comment != "" {
			param.Comment = cloudflare.F(obj.Comment)
		}
		return param, nil

	case "PTR":
		param := dns.PTRRecordParam{
			Name:    cloudflare.F(obj.RecordName),
			Type:    cloudflare.F(dns.PTRRecordTypePTR),
			Content: cloudflare.F(obj.Content),
			TTL:     cloudflare.F(ttl),
		}
		if obj.Proxied != nil {
			param.Proxied = cloudflare.F(*obj.Proxied)
		}
		if obj.Comment != "" {
			param.Comment = cloudflare.F(obj.Comment)
		}
		return param, nil

	default:
		return nil, fmt.Errorf("record type %s is not supported", obj.Type)

	}
}

// buildNewRecordParam creates the appropriate record parameter for creating new
// records.
func (obj *CloudflareDNSRes) buildNewRecordParam() (dns.RecordNewParamsBodyUnion, error) {
	result, err := obj.buildRecordParam()
	if err != nil {
		return nil, err
	}
	return result.(dns.RecordNewParamsBodyUnion), nil
}

// buildEditRecordParam creates the appropriate record parameter for editing
// records.
func (obj *CloudflareDNSRes) buildEditRecordParam() (dns.RecordEditParamsBodyUnion, error) {
	result, err := obj.buildRecordParam()
	if err != nil {
		return nil, err
	}
	return result.(dns.RecordEditParamsBodyUnion), nil
}

// createRecord creates a new DNS record in Cloudflare using the resource's
// parameters.
func (obj *CloudflareDNSRes) createRecord(ctx context.Context) error {
	recordParams, err := obj.buildNewRecordParam()
	if err != nil {
		return err
	}

	createParams := dns.RecordNewParams{
		ZoneID: cloudflare.F(obj.zoneID),
		Body:   recordParams,
	}

	_, err = obj.client.DNS.Records.New(ctx, createParams)
	if err != nil {
		return errwrap.Wrapf(err, "failed to create dns record")
	}

	return nil
}

// updateRecord updates an existing DNS record in Cloudflare with the resource's
// parameters.
func (obj *CloudflareDNSRes) updateRecord(ctx context.Context, recordID string) error {
	recordParams, err := obj.buildEditRecordParam()
	if err != nil {
		return err
	}

	editParams := dns.RecordEditParams{
		ZoneID: cloudflare.F(obj.zoneID),
		Body:   recordParams,
	}

	_, err = obj.client.DNS.Records.Edit(ctx, recordID, editParams)
	if err != nil {
		return errwrap.Wrapf(err, "failed to update dns record")
	}

	return nil
}

// needsUpdate compares the current DNS record with the desired state and
// returns true if an update is needed.
func (obj *CloudflareDNSRes) needsUpdate(record dns.RecordResponse) bool {
	if obj.Content != record.Content {
		return true
	}

	if obj.TTL != int64(record.TTL) {
		return true
	}

	if obj.Proxied != nil {
		if *obj.Proxied != record.Proxied {
			return true
		}
	}

	if obj.Priority != nil {
		if float64(*obj.Priority) != record.Priority {
			return true
		}
	}

	if obj.Comment != "" && obj.Comment != record.Comment {
		return true
	}

	//TODO: add more checks?

	return false

}

// purgeCheckApply deletes all DNS records in the zone that are not defined in
// the mgmt graph. It queries the graph for other cloudflare:dns resources in
// the same zone and builds an exclusion list. If apply is false, it only checks
// if purge is needed.
func (obj *CloudflareDNSRes) purgeCheckApply(ctx context.Context, apply bool) (bool, error) {
	listParams := dns.RecordListParams{
		ZoneID: cloudflare.F(obj.zoneID),
	}

	iter := obj.client.DNS.Records.ListAutoPaging(ctx, listParams)

	allRecords := []dns.RecordResponse{}
	for iter.Next() {
		allRecords = append(allRecords, iter.Current())
	}
	if err := iter.Err(); err != nil {
		return false, errwrap.Wrapf(err, "failed to list dns records for purge")
	}

	excludes := make(map[string]bool)

	graph, err := obj.init.FilteredGraph()
	if err != nil {
		return false, errwrap.Wrapf(err, "can't read the filtered graph")
	}

	for _, vertex := range graph.Vertices() {
		res, ok := vertex.(engine.Res)
		if !ok {
			return false, fmt.Errorf("not a resource")
		}
		if res.Kind() != "cloudflare:dns" {
			continue // we only want cloudflare dns resources
		}
		if res.Name() == obj.Name() {
			continue // skip self
		}

		cfRes, ok := res.(*CloudflareDNSRes)
		if !ok {
			return false, fmt.Errorf("wrong resource type")
		}

		if cfRes.Zone == obj.Zone {
			recordKey := fmt.Sprintf("%s:%s:%s", cfRes.RecordName, cfRes.Type,
				cfRes.Content)
			if cfRes.Priority != nil {
				// corner case for MX records which require priority set
				recordKey = fmt.Sprintf("%s:%d", recordKey, *cfRes.Priority)
			}
			excludes[recordKey] = true
		}
	}

	checkOK := true

	for _, record := range allRecords {
		recordKey := fmt.Sprintf("%s:%s:%s", record.Name, record.Type,
			record.Content)
		if record.Priority != 0 {
			recordKey = fmt.Sprintf("%s:%g", recordKey, record.Priority)
		}

		if excludes[recordKey] {
			continue
		}

		if apply {
			deleteParams := dns.RecordDeleteParams{
				ZoneID: cloudflare.F(obj.zoneID),
			}
			_, err := obj.client.DNS.Records.Delete(ctx, record.ID, deleteParams)
			if err != nil {
				return false, errwrap.Wrapf(err, "failed to purge %s", recordKey)
			}
			obj.init.Logf("purged unmanaged DNS records: %s", recordKey)
		} else {
			checkOK = false
		}
	}

	return checkOK, nil
}

// GraphQueryAllowed returns nil if you're allowed to query the graph. This
// function accepts information about the requesting resource so we can
// determine the access with some form of fine-grained control.
func (obj *CloudflareDNSRes) GraphQueryAllowed(opts ...engine.GraphQueryableOption) error {
	options := &engine.GraphQueryableOptions{} // default options
	options.Apply(opts...)                     // apply the options
	if options.Kind != "cloudflare:dns" {
		return fmt.Errorf("only other cloudflare dns resources can access this info")
	}
	return nil
}

// matchesRecordName checks if a record name from the API matches our desired
// record name. Handles both FQDN (www.example.com) and short form (www)
// comparisons.
func (obj *CloudflareDNSRes) matchesRecordName(apiRecordName string) bool {
	desired := obj.normalizeRecordName(obj.RecordName)
	actual := obj.normalizeRecordName(apiRecordName)
	return desired == actual
}

// normalizeRecordName converts a record name to a consistent format for
// comparison. Converts to FQDN format (e.g., "www" -> "www.example.com", "@" ->
// "example.com")
func (obj *CloudflareDNSRes) normalizeRecordName(name string) string {
	if name == "@" || name == obj.Zone {
		return obj.Zone
	}

	if strings.HasSuffix(name, "."+obj.Zone) {
		return name
	}

	return name + "." + obj.Zone
}

// lookupRecordViaDNS queries the DNS records to check if they exists and
// matches the desired content. Returns (exists, matches, error): `exists`
// returns true if any record of this type exists for this name; `matches`
// returns true if the record matches our desired content; `error` returns any
// DNS lookup error.
func (obj *CloudflareDNSRes) lookupRecordViaDNS(ctx context.Context) (bool, bool, error) {
	r := &net.Resolver{}
	recordName := obj.normalizeRecordName(obj.RecordName)

	switch obj.Type {
	case "A":
		return obj.lookupARecord(ctx, r, recordName)
	case "AAAA":
		return obj.lookupAAAARecord(ctx, r, recordName)
	case "CNAME":
		return obj.lookupCNAMERecord(ctx, r, recordName)
	case "TXT":
		return obj.lookupTXTRecord(ctx, r, recordName)
	case "MX":
		return obj.lookupMXRecord(ctx, r, recordName)
	case "NS":
		return obj.lookupNSRecord(ctx, r, recordName)
	case "PTR":
		return obj.lookupPTRRecord(ctx, r, recordName)
	default:
		return false, false, fmt.Errorf("dns lookup not support for type %s", obj.Type)
	}
}

// lookupARecord performs a DNS lookup for A records
func (obj *CloudflareDNSRes) lookupARecord(ctx context.Context, r *net.Resolver, recordName string) (bool, bool, error) {
	ips, err := r.LookupNetIP(ctx, "ip4", recordName)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return false, false, nil // record doesn't exist
		}
		return false, false, err // lookup error
	}

	if len(ips) == 0 {
		return false, false, nil // record doesn't exist
	}

	desiredIP := net.ParseIP(obj.Content)
	if desiredIP == nil {
		return false, false, fmt.Errorf("invalid IP address: %s", obj.Content)
	}

	for _, ip := range ips {
		if ip.String() == desiredIP.String() {
			return true, true, nil // exists and matches
		}
	}

	return true, false, nil // exists but doesn't match
}

// lookupAAAARecord performs a DNS lookup for AAAA records
func (obj *CloudflareDNSRes) lookupAAAARecord(ctx context.Context, r *net.Resolver, recordName string) (bool, bool, error) {
	ips, err := r.LookupNetIP(ctx, "ip6", recordName)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return false, false, nil // record doesn't exist
		}
		return false, false, err // lookup error
	}

	if len(ips) == 0 {
		return false, false, nil // record doesn't exist
	}

	desiredIP := net.ParseIP(obj.Content)
	if desiredIP == nil {
		return false, false, fmt.Errorf("invalid IP address: %s", obj.Content)
	}

	for _, ip := range ips {
		if ip.String() == desiredIP.String() {
			return true, true, nil // exists and matches
		}
	}

	return true, false, nil // exists but doesn't match
}

// lookupCNAMERecord performs a DNS lookup for CNAME records
func (obj *CloudflareDNSRes) lookupCNAMERecord(ctx context.Context, r *net.Resolver, recordName string) (bool, bool, error) {
	cname, err := r.LookupCNAME(ctx, recordName)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return false, false, nil // record doesn't exist
		}
		return false, false, err // lookup error
	}

	// CNAME record normalization
	cname = strings.TrimSuffix(cname, ".")
	desired := strings.TrimSuffix(obj.Content, ".")

	if cname == desired {
		return true, true, nil // exists and matches
	}

	return true, false, nil // exists but doesn't match
}

// lookupTXTRecord performs a DNS lookup for TXT records
func (obj *CloudflareDNSRes) lookupTXTRecord(ctx context.Context, r *net.Resolver, recordName string) (bool, bool, error) {
	txts, err := r.LookupTXT(ctx, recordName)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return false, false, nil // record doesn't exist
		}
		return false, false, err // lookup error
	}

	if len(txts) == 0 {
		return false, false, nil // record doesn't exist
	}

	// Check if the TXT record we want is in the results
	for _, txt := range txts {
		// first we have to deal with corner case where the DNS response comes
		// without quotation marks, but the content needs to be in quotation
		// marks
		if strings.HasPrefix(obj.Content, "\"") && strings.HasSuffix(obj.Content, "\"") {
			trimmed := obj.Content
			trimmed = strings.TrimPrefix(trimmed, "\"")
			trimmed = strings.TrimSuffix(trimmed, "\"")
			if txt == trimmed {
				return true, true, nil // exists and matched
			}
		}
		if txt == obj.Content {
			return true, true, nil // exists and matches
		}
	}

	return true, false, nil // exists but doesn't match
}

// lookupMXRecord performs a DNS lookup for MX records
func (obj *CloudflareDNSRes) lookupMXRecord(ctx context.Context, r *net.Resolver, recordName string) (bool, bool, error) {
	mxs, err := r.LookupMX(ctx, recordName)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return false, false, nil // record doesn't exist
		}
		return false, false, err // lookup error
	}

	if len(mxs) == 0 {
		return false, false, nil // record doesn't exist
	}

	// normalize MX host
	desired := strings.TrimSuffix(obj.Content, ".")

	// Check if the MX record we want is in the results
	for _, mx := range mxs {
		host := strings.TrimSuffix(mx.Host, ".")
		if host == desired {
			// need to account for priority with MX records
			if obj.Priority != nil && uint16(*obj.Priority) != mx.Pref {
				return true, false, nil // exists but doesn't match
			}
			return true, true, nil // exists and matches
		}
	}

	return true, false, nil // exists but doesn't match
}

// lookupNSRecord performs a DNS lookup for NS records
func (obj *CloudflareDNSRes) lookupNSRecord(ctx context.Context, r *net.Resolver, recordName string) (bool, bool, error) {
	nss, err := r.LookupNS(ctx, recordName)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return false, false, nil // record doesn't exist
		}
		return false, false, err // lookup error
	}

	if len(nss) == 0 {
		return false, false, nil // record doesn't exist
	}

	// normalize NS host
	desired := strings.TrimSuffix(obj.Content, ".")

	// Check if the NS record we want is in the results
	for _, ns := range nss {
		host := strings.TrimSuffix(ns.Host, ".")
		if host == desired {
			return true, true, nil // exists and matches
		}
	}

	return true, false, nil // exists but doesn't match
}

// lookupPTRRecord performs a DNS lookup for PTR records
func (obj *CloudflareDNSRes) lookupPTRRecord(ctx context.Context, r *net.Resolver, recordName string) (bool, bool, error) {
	// we assume that the address is already in the correct format
	ptrs, err := r.LookupAddr(ctx, recordName)
	if err != nil {
		if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
			return false, false, nil // record doesn't exist
		}
		return false, false, err // lookup error
	}

	if len(ptrs) == 0 {
		return false, false, nil // record doesn't exist
	}

	// normalize PTR host
	desired := strings.TrimSuffix(obj.Content, ".")

	// Check if the PTR record we want is in the results
	for _, ptr := range ptrs {
		host := strings.TrimSuffix(ptr, ".")
		if host == desired {
			return true, true, nil // exists and matches
		}
	}

	return true, false, nil // exists but doesn't match
}

// needsMetadataUpdate checks if only metadata fields differ. This is useful to
// decide if, when DNS content matches, we still need an API call
func (obj *CloudflareDNSRes) needsMetadataUpdate() bool {
	if obj.TTL != 1 {
		return true
	}
	if obj.Proxied != nil {
		return true
	}
	if obj.Comment != "" {
		return true
	}
	return false
}

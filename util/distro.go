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

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const (
	// FedoraDownloadURL is the default fedora releases and updates download
	// URL. If you try to use this, you will get redirected to a mirror near
	// you.
	FedoraDownloadURL = "https://download.fedoraproject.org/pub/fedora/linux/"

	// FedoraReleasesEndpointJSON is the location of the fedora release data
	// as specified in json format.
	FedoraReleasesEndpointJSON = "https://fedoraproject.org/releases.json"
)

// GetFedoraDownloadURL gets an https base path of a mirror to use for
// downloading both the fedora release and updates. It is supposed to find
// something nearest to you.
// TODO: Do we need to specify version and arch to make sure mirror has those?
func GetFedoraDownloadURL(ctx context.Context) (string, error) {
	var out *url.URL
	checkRedirectFunc := func(req *http.Request, via []*http.Request) error {
		out = req.URL
		if len(via) > 1 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}
	client := &http.Client{
		CheckRedirect: checkRedirectFunc,
	}
	req, err := http.NewRequestWithContext(ctx, "GET", FedoraDownloadURL, nil)
	if err != nil {
		return "", err
	}
	result, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer result.Body.Close()

	if out == nil {
		return "", fmt.Errorf("no url found")
	}
	if out.Scheme != "https" {
		return "", fmt.Errorf("bad scheme, got: %s", out.Scheme)
	}

	return out.String(), nil
}

// FedoraRelease is a partial struct of the available data in the json releases
// file found. This was determined by inspection.
type FedoraRelease struct {
	Version string `json:"version"`
	Arch    string `json:"arch"`
	Variant string `json:"variant"`
}

// LatestFedoraVersion returns the version number (as a string) of the latest
// fedora release known. This looks at a well-known endpoint to get the value.
// If you specify a non-empty arch, it will filter to that.
func LatestFedoraVersion(ctx context.Context, arch string) (string, error) {
	client := &http.Client{}

	req, err := http.NewRequestWithContext(ctx, "GET", FedoraReleasesEndpointJSON, nil)
	if err != nil {
		return "", err
	}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	// XXX: We should stream decode this instead.
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	var data []FedoraRelease
	if err := json.Unmarshal(b, &data); err != nil {
		return "", err
	}

	//fmt.Printf("len: %d\n", len(data))
	m := 0
	for _, r := range data {
		version, err := strconv.Atoi(r.Version)
		if err != nil { // skip strings like "40 Beta"
			continue
		}
		if arch != "" && arch != r.Arch { // skip others
			continue
		}

		m = max(m, version)
	}
	if m == 0 {
		if arch == "" {
			return "", fmt.Errorf("no versions found")
		}
		return "", fmt.Errorf("no versions found with arch: %s", arch)
	}
	//fmt.Printf("max: %d\n", m)
	return strconv.Itoa(m), nil
}

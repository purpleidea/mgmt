// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package etcd

import (
	"fmt"
	"strconv"
	"strings"

	etcd "github.com/coreos/etcd/clientv3"
	errwrap "github.com/pkg/errors"
)

const (
	deployPath  = "deploy"
	payloadPath = "payload"
	hashPath    = "hash"
)

// WatchDeploy returns a channel which spits out events on new deploy activity.
// FIXME: It should close the channel when it's done, and spit out errors when
// something goes wrong.
func WatchDeploy(obj *EmbdEtcd) chan error {
	// key structure is $NS/deploy/$id/payload = $data
	path := fmt.Sprintf("%s/%s/", NS, deployPath)
	ch := make(chan error, 1)
	// FIXME: fix our API so that we get a close event on shutdown.
	callback := func(re *RE) error {
		// TODO: is this even needed? it used to happen on conn errors
		//log.Printf("Etcd: Watch: Path: %v", path) // event
		if re == nil || re.response.Canceled {
			return fmt.Errorf("watch is empty") // will cause a CtxError+retry
		}
		if len(ch) == 0 { // send event only if one isn't pending
			ch <- nil // event
		}
		return nil
	}
	_, _ = obj.AddWatcher(path, callback, true, false, etcd.WithPrefix()) // no need to check errors
	return ch
}

// GetDeploys gets all the available deploys.
func GetDeploys(obj Client) (map[uint64]string, error) {
	// key structure is $NS/deploy/$id/payload = $data
	path := fmt.Sprintf("%s/%s/", NS, deployPath)
	keyMap, err := obj.Get(path, etcd.WithPrefix(), etcd.WithSort(etcd.SortByKey, etcd.SortAscend))
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not get deploy")
	}
	result := make(map[uint64]string)
	for key, val := range keyMap {
		if !strings.HasPrefix(key, path) { // sanity check
			continue
		}

		str := strings.Split(key[len(path):], "/")
		if len(str) != 2 {
			return nil, fmt.Errorf("unexpected chunk count of %d", len(str))
		}
		if s := str[1]; s != payloadPath {
			continue // skip, maybe there are other future additions
		}

		var id uint64
		var err error
		x := str[0]
		if id, err = strconv.ParseUint(x, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid id of `%s`", x)
		}

		// TODO: do some sort of filtering here?
		//log.Printf("Etcd: GetDeploys(%s): Id => Data: %d => %s", key, id, val)
		result[id] = val
	}
	return result, nil
}

// calculateMax is a helper function.
func calculateMax(deploys map[uint64]string) uint64 {
	var max uint64
	for i := range deploys {
		if i > max {
			max = i
		}
	}
	return max
}

// GetDeploy returns the deploy with the specified id if it exists. If you input
// an id of 0, you'll get back an empty deploy without error. This is useful so
// that you can pass through this function easily.
// FIXME: implement this more efficiently so that it doesn't have to download *all* the old deploys from etcd!
func GetDeploy(obj Client, id uint64) (string, error) {
	result, err := GetDeploys(obj)
	if err != nil {
		return "", err
	}

	// don't optimize this test to the top, because it's better to catch an
	// etcd failure early if we can, rather than fail later when we deploy!
	if id == 0 {
		return "", nil // no results yet
	}

	str, exists := result[id]
	if !exists {
		return "", fmt.Errorf("can't find id `%d`", id)
	}
	return str, nil
}

// GetMaxDeployID returns the maximum deploy id. If none are found, this returns
// zero. You must increment the returned value by one when you add a deploy. If
// two or more clients race for this deploy id, then the loser is not committed,
// and must repeat this GetMaxDeployID process until it succeeds with a commit!
func GetMaxDeployID(obj Client) (uint64, error) {
	// TODO: this was all implemented super inefficiently, fix up for perf!
	deploys, err := GetDeploys(obj) // get previous deploys
	if err != nil {
		return 0, errwrap.Wrapf(err, "error getting previous deploys")
	}
	// find the latest id
	max := calculateMax(deploys)
	return max, nil // found! (or zero)
}

// AddDeploy adds a new deploy. It takes an id and ensures it's sequential. If
// hash is not empty, then it will check that the pHash matches what the
// previous hash was, and also adds this new hash along side the id. This is
// useful to make sure you get a linear chain of git patches, and to avoid two
// contributors pushing conflicting deploys. This isn't git specific, and so any
// arbitrary string hash can be used.
// FIXME: prune old deploys from the store when they aren't needed anymore...
func AddDeploy(obj Client, id uint64, hash, pHash string, data *string) error {
	// key structure is $NS/deploy/$id/payload = $data
	// key structure is $NS/deploy/$id/hash = $hash
	path := fmt.Sprintf("%s/%s/%d/%s", NS, deployPath, id, payloadPath)
	tPath := fmt.Sprintf("%s/%s/%d/%s", NS, deployPath, id, hashPath)
	ifs := []etcd.Cmp{} // list matching the desired state
	ops := []etcd.Op{}  // list of ops in this transaction (then)

	// TODO: use https://github.com/coreos/etcd/pull/7417 if merged
	// we're append only, so ensure this unique deploy id doesn't exist
	ifs = append(ifs, etcd.Compare(etcd.Version(path), "=", 0)) // KeyMissing
	//ifs = append(ifs, etcd.KeyMissing(path))

	// don't look for previous deploy if this is the first deploy ever
	if id > 1 {
		// we append sequentially, so ensure previous key *does* exist
		prev := fmt.Sprintf("%s/%s/%d/%s", NS, deployPath, id-1, payloadPath)
		ifs = append(ifs, etcd.Compare(etcd.Version(prev), ">", 0)) // KeyExists
		//ifs = append(ifs, etcd.KeyExists(prev))

		if hash != "" && pHash != "" {
			// does the previously stored hash match what we expect?
			prevHash := fmt.Sprintf("%s/%s/%d/%s", NS, deployPath, id-1, hashPath)
			ifs = append(ifs, etcd.Compare(etcd.Value(prevHash), "=", pHash))
		}
	}

	ops = append(ops, etcd.OpPut(path, *data))
	if hash != "" {
		ops = append(ops, etcd.OpPut(tPath, hash)) // store new hash as well
	}

	// it's important to do this in one transaction, and atomically, because
	// this way, we only generate one watch event, and only when it's needed
	result, err := obj.Txn(ifs, ops, nil)
	if err != nil {
		return errwrap.Wrapf(err, "error creating deploy id %d", id)
	}
	if !result.Succeeded {
		return fmt.Errorf("could not create deploy id %d", id)
	}
	return nil // success
}

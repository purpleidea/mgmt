// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package pgp

import (
	"bufio"
	"bytes"
	"crypto"
	"encoding/base64"
	"io/ioutil"
	"log"
	"os"
	"strings"

	errwrap "github.com/pkg/errors"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/packet"
)

// DefaultKeyringFile is the default file name for keyrings.
const DefaultKeyringFile = "keyring.pgp"

// CONFIG set default Hash.
var CONFIG packet.Config

func init() {
	CONFIG.DefaultHash = crypto.SHA256
}

// PGP contains base entity.
type PGP struct {
	Entity *openpgp.Entity
}

// Import private key from defined path.
func Import(privKeyPath string) (*PGP, error) {

	privKeyFile, err := os.Open(privKeyPath)
	if err != nil {
		return nil, err
	}
	defer privKeyFile.Close()

	file := packet.NewReader(bufio.NewReader(privKeyFile))
	entity, err := openpgp.ReadEntity(file)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read entity from path")
	}

	obj := &PGP{
		Entity: entity,
	}

	log.Printf("PGP: Imported key: %s", obj.Entity.PrivateKey.KeyIdShortString())
	return obj, nil
}

// Generate creates new key pair. This key pair must be saved or it will be lost.
func Generate(name, comment, email string, hash *crypto.Hash) (*PGP, error) {
	if hash != nil {
		CONFIG.DefaultHash = *hash
	}
	// generate a new public/private key pair
	entity, err := openpgp.NewEntity(name, comment, email, &CONFIG)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't generate entity")
	}

	obj := &PGP{
		Entity: entity,
	}

	log.Printf("PGP: Created key: %s", obj.Entity.PrivateKey.KeyIdShortString())
	return obj, nil
}

// SaveKey writes the whole entity (including private key!) to a .gpg file.
func (obj *PGP) SaveKey(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return errwrap.Wrapf(err, "can't create file from given path")
	}

	w := bufio.NewWriter(f)
	if err != nil {
		return errwrap.Wrapf(err, "can't create writer")
	}

	if err := obj.Entity.SerializePrivate(w, &CONFIG); err != nil {
		return errwrap.Wrapf(err, "can't serialize private key")
	}

	for _, ident := range obj.Entity.Identities {
		for _, sig := range ident.Signatures {
			if err := sig.Serialize(w); err != nil {
				return errwrap.Wrapf(err, "can't serialize signature")
			}
		}
	}

	if err := w.Flush(); err != nil {
		return errwrap.Wrapf(err, "enable to flush writer")
	}

	return nil
}

// WriteFile from given buffer in specified path.
func (obj *PGP) WriteFile(path string, buff *bytes.Buffer) error {
	w, err := createWriter(path)
	if err != nil {
		return errwrap.Wrapf(err, "can't create writer")
	}
	buff.WriteTo(w)

	if err := w.Flush(); err != nil {
		return errwrap.Wrapf(err, "can't flush buffered data")
	}
	return nil
}

// CreateWriter remove duplicate function.
func createWriter(path string) (*bufio.Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't create file from given path")
	}
	return bufio.NewWriter(f), nil
}

// Encrypt message for specified entity.
func (obj *PGP) Encrypt(to *openpgp.Entity, msg string) (string, error) {
	buf, err := obj.EncryptMsg(to, msg)
	if err != nil {
		return "", errwrap.Wrapf(err, "can't encrypt message")
	}

	// encode to base64
	bytes, err := ioutil.ReadAll(buf)
	if err != nil {
		return "", errwrap.Wrapf(err, "can't read unverified body")
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// EncryptMsg encrypts the message.
func (obj *PGP) EncryptMsg(to *openpgp.Entity, msg string) (*bytes.Buffer, error) {
	ents := []*openpgp.Entity{to}

	buf := new(bytes.Buffer)
	w, err := openpgp.Encrypt(buf, ents, obj.Entity, nil, nil)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't encrypt message")
	}

	_, err = w.Write([]byte(msg))
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't write to buffer")
	}

	if err = w.Close(); err != nil {
		return nil, errwrap.Wrapf(err, "can't close writer")
	}
	return buf, nil
}

// Decrypt an encrypted msg.
func (obj *PGP) Decrypt(encString string) (string, error) {
	entityList := openpgp.EntityList{obj.Entity}

	// decode the base64 string
	dec, err := base64.StdEncoding.DecodeString(encString)
	if err != nil {
		return "", errwrap.Wrapf(err, "fail at decoding encrypted string")
	}

	// decrypt it with the contents of the private key
	md, err := openpgp.ReadMessage(bytes.NewBuffer(dec), entityList, nil, nil)
	if err != nil {
		return "", errwrap.Wrapf(err, "can't read message")
	}

	bytes, err := ioutil.ReadAll(md.UnverifiedBody)
	if err != nil {
		return "", errwrap.Wrapf(err, "can't read unverified body")
	}
	return string(bytes), nil
}

// GetIdentities return the first identities from current object.
func (obj *PGP) GetIdentities() (string, error) {
	identities := []*openpgp.Identity{}

	for _, v := range obj.Entity.Identities {
		identities = append(identities, v)
	}
	return identities[0].Name, nil
}

// ParseIdentity parses an identity into name, comment and email components.
func ParseIdentity(identity string) (name, comment, email string, err error) {
	// get name
	n := strings.Split(identity, " <")
	if len(n) != 2 {
		return "", "", "", errwrap.Wrap(err, "user string mal formated")
	}

	// get email and comment
	ec := strings.Split(n[1], "> ")
	if len(ec) != 2 {
		return "", "", "", errwrap.Wrap(err, "user string mal formated")
	}

	return n[0], ec[1], ec[0], nil
}

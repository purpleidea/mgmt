// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
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

package gpg

import (
	"bufio"
	"bytes"
	"crypto"
	"encoding/base64"
	"encoding/gob"
	"io/ioutil"
	"log"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/packet"
)

func init() {
	gob.Register(&GpgRes{})
}

// GpgRes is a no-op resource that does nothing.
type GpgRes struct {
	Name    string `yaml:"name"`
	Comment string `yaml:"comment"` // extra field for example purposes
	Email   string `yaml:"email"`
	Entity  *openpgp.Entity
}

// NewGpgRes is a constructor for this resource. It also calls Init() for you.
func NewGpgRes(name string, email string) *GpgRes {
	obj := &GpgRes{
		Name:    name,
		Comment: "",
		Email:   email,
	}
	obj.Init()
	return obj
}

// Init runs some startup code for this resource.
func (obj *GpgRes) Init() {
	log.Println("TESTING GPG")

	var err error
	var config packet.Config
	config.DefaultHash = crypto.SHA256

	obj.Entity, err = openpgp.NewEntity(obj.Name, obj.Comment, obj.Email, &config)
	if err != nil {
		log.Println(err)
		return
	}
	// obj.savePubKey()
}

func WriteKeyRing() {

}

func (obj *GpgRes) SavePubKey() {
	log.Println("Save Public key")

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	e := obj.Entity

	err := e.Serialize(w)
	if err != nil {
		log.Println(err)
	}

}

// TODO Get all present PubKey and add to SubKey
// addSubKey
func (obj *GpgRes) AddSubKey(pubKey packet.PublicKey) {
	// sub := &obj.Entity.Subkeys
	// var key openpgp.Key
	var subKeys []openpgp.Subkey
	// subKeys
	obj.Entity.Subkeys = subKeys
}

// TODO Remove any save file for the current entity (ex : PubKey)
func (obj *GpgRes) cleanAll() {

}

// Crypt a msg then return the encString
func (obj *GpgRes) Crypt(to *openpgp.Entity, msg string) string {
	ents := []*openpgp.Entity{to}
	// ents := obj.Entity.Subkeys

	log.Println("Crypting the test file")

	buf := new(bytes.Buffer)
	w, err := openpgp.Encrypt(buf, ents, obj.Entity, nil, nil)
	checkError(err)

	_, err = w.Write([]byte(msg))
	checkError(err)

	err = w.Close()
	checkError(err)

	// Encode to base64
	bytes, err := ioutil.ReadAll(buf)
	checkError(err)
	encString := base64.StdEncoding.EncodeToString(bytes)
	// Output encrypted/encoded string
	log.Println("Encrypted Secret:", encString)

	return encString
}

// Decrypt a encrypted msg
func (obj *GpgRes) Decrypt(encString string) string {
	entityList := openpgp.EntityList{obj.Entity}
	log.Println("Decrypting the test file")

	// Decode the base64 string
	dec, err := base64.StdEncoding.DecodeString(encString)
	checkError(err)

	// Decrypt it with the contents of the private key
	md, err := openpgp.ReadMessage(bytes.NewBuffer(dec), entityList, nil, nil)
	checkError(err)

	bytes, err := ioutil.ReadAll(md.UnverifiedBody)
	checkError(err)

	return string(bytes)
}

func checkError(err error) {
	if err != nil {
		log.Println(err)
	}
}

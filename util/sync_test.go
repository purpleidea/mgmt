// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

//go:build !root

package util

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestEasyAck1(t *testing.T) {
	ea := NewEasyAck()
	ea.Ack() // send the ack
	select {
	case <-ea.Wait(): // we got it!
	case <-time.After(time.Duration(60) * time.Second):
		t.Errorf("the Ack did not arrive in time")
	}
}

func TestEasyAck2(t *testing.T) {
	ea := NewEasyAck()
	// never send an ack
	select {
	case <-ea.Wait(): // we got it!
		t.Errorf("the Ack arrived unexpectedly")
	default:
	}
}

func TestEasyAck3(t *testing.T) {
	ea := NewEasyAck()
	ea.Ack() // send the ack
	select {
	case <-ea.Wait(): // we got it!
	case <-time.After(time.Duration(60) * time.Second):
		t.Errorf("the Ack did not arrive in time")
	}

	ea = NewEasyAck() // build a new one
	ea.Ack()          // send the ack
	select {
	case <-ea.Wait(): // we got it!
	case <-time.After(time.Duration(60) * time.Second):
		t.Errorf("the second Ack did not arrive in time")
	}
}

func TestEasyAckOnce1(t *testing.T) {
	eao := NewEasyAckOnce()
	eao.Ack()
	eao.Ack() // must not panic
	eao.Ack()
	select {
	case <-eao.Wait(): // we got it!
	case <-time.After(time.Duration(60) * time.Second):
		t.Errorf("the Ack did not arrive in time")
	}
}

func TestEasyAckOnce2(t *testing.T) {
	eao := NewEasyAckOnce()
	// never send an ack
	select {
	case <-eao.Wait(): // we got it!
		t.Errorf("the Ack arrived unexpectedly")
	default:
	}
}

func ExampleSubscribedSignal() {
	fmt.Println("hello")

	x := &SubscribedSignal{}
	wg := &sync.WaitGroup{}
	ready := &sync.WaitGroup{}

	// unit1
	wg.Add(1)
	ready.Add(1)
	go func() {
		defer wg.Done()
		ch, ack := x.Subscribe()
		ready.Done()
		select {
		case <-ch:
			fmt.Println("got signal")
		}
		time.Sleep(1 * time.Second) // wait a bit for fun
		fmt.Println("(1) sending ack...")
		ack() // must call ack
		fmt.Println("done sending ack")
	}()

	// unit2
	wg.Add(1)
	ready.Add(1)
	go func() {
		defer wg.Done()
		ch, ack := x.Subscribe()
		ready.Done()
		select {
		case <-ch:
			fmt.Println("got signal")
		}
		time.Sleep(2 * time.Second) // wait a bit for fun
		fmt.Println("(2) sending ack...")
		ack() // must call ack
		fmt.Println("done sending ack")
	}()

	// unit3
	wg.Add(1)
	ready.Add(1)
	go func() {
		defer wg.Done()
		ch, ack := x.Subscribe()
		ready.Done()
		select {
		case <-ch:
			fmt.Println("got signal")
		}
		time.Sleep(3 * time.Second) // wait a bit for fun
		fmt.Println("(3) sending ack...")
		ack() // must call ack
		fmt.Println("done sending ack")
	}()

	ready.Wait() // wait for all subscribes
	fmt.Println("sending signal...")
	x.Send()                    // trigger!
	time.Sleep(1 * time.Second) // wait a bit so the next print doesn't race
	fmt.Println("done sending signal")

	wg.Wait() // wait for everyone to exit
	fmt.Println("exiting...")

	// Output: hello
	// sending signal...
	// got signal
	// got signal
	// got signal
	// (1) sending ack...
	// (2) sending ack...
	// (3) sending ack...
	// done sending ack
	// done sending ack
	// done sending ack
	// done sending signal
	// exiting...
}

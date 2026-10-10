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

//go:build !noaugeas && !augeas_dlopen

package augeas

// #cgo pkg-config: libxml-2.0 augeas
// #include <augeas.h>
//
// // The library is linked, so we can call it directly.
// const char *mgmt_augeas_library(void) { return NULL; }
// augeas *mgmt_aug_init(const char *root, const char *loadpath, unsigned int flags) { return aug_init(root, loadpath, flags); }
// int mgmt_aug_get(const augeas *aug, const char *path, const char **value) { return aug_get(aug, path, value); }
// int mgmt_aug_set(augeas *aug, const char *path, const char *value) { return aug_set(aug, path, value); }
// int mgmt_aug_load(augeas *aug) { return aug_load(aug); }
// int mgmt_aug_save(augeas *aug) { return aug_save(aug); }
// void mgmt_aug_close(augeas *aug) { aug_close(aug); }
// int mgmt_aug_error(augeas *aug) { return aug_error(aug); }
// const char *mgmt_aug_error_message(augeas *aug) { return aug_error_message(aug); }
// const char *mgmt_aug_error_minor_message(augeas *aug) { return aug_error_minor_message(aug); }
// const char *mgmt_aug_error_details(augeas *aug) { return aug_error_details(aug); }
import "C"

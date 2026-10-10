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

//go:build !noaugeas && augeas_dlopen

package augeas

// #cgo LDFLAGS: -ldl
// #include <dlfcn.h>
// #include <stddef.h>
//
// // We don't include augeas.h, so that we don't need it to build. These match
// // the declarations in it, and we only ever use augeas as an opaque pointer.
// typedef struct augeas augeas;
// static augeas *(*aug_init_fn)(const char *, const char *, unsigned int);
// static int (*aug_get_fn)(const augeas *, const char *, const char **);
// static int (*aug_set_fn)(augeas *, const char *, const char *);
// static int (*aug_load_fn)(augeas *);
// static int (*aug_save_fn)(augeas *);
// static void (*aug_close_fn)(augeas *);
// static int (*aug_error_fn)(augeas *);
// static const char *(*aug_error_message_fn)(augeas *);
// static const char *(*aug_error_minor_message_fn)(augeas *);
// static const char *(*aug_error_details_fn)(augeas *);
//
// #define MGMT_AUGEAS_SYMBOL(handle, name) \
// 	*(void **)(&name##_fn) = dlsym(handle, #name); \
// 	if (name##_fn == NULL) { \
// 		return "missing symbol: " #name; \
// 	}
//
// // This loads the library and returns NULL, or an error message on failure.
// // It is only ever called once, and before anything else, by Available. On
// // failure, we leave the handle open, since we won't use it, or try again.
// const char *mgmt_augeas_library(void) {
// 	void *handle = dlopen("libaugeas.so.0", RTLD_NOW|RTLD_LOCAL);
// 	if (handle == NULL) {
// 		return dlerror();
// 	}
// 	MGMT_AUGEAS_SYMBOL(handle, aug_init)
// 	MGMT_AUGEAS_SYMBOL(handle, aug_get)
// 	MGMT_AUGEAS_SYMBOL(handle, aug_set)
// 	MGMT_AUGEAS_SYMBOL(handle, aug_load)
// 	MGMT_AUGEAS_SYMBOL(handle, aug_save)
// 	MGMT_AUGEAS_SYMBOL(handle, aug_close)
// 	MGMT_AUGEAS_SYMBOL(handle, aug_error)
// 	MGMT_AUGEAS_SYMBOL(handle, aug_error_message)
// 	MGMT_AUGEAS_SYMBOL(handle, aug_error_minor_message)
// 	MGMT_AUGEAS_SYMBOL(handle, aug_error_details)
// 	return NULL;
// }
//
// // These are only called after mgmt_augeas_library succeeded.
// augeas *mgmt_aug_init(const char *root, const char *loadpath, unsigned int flags) { return aug_init_fn(root, loadpath, flags); }
// int mgmt_aug_get(const augeas *aug, const char *path, const char **value) { return aug_get_fn(aug, path, value); }
// int mgmt_aug_set(augeas *aug, const char *path, const char *value) { return aug_set_fn(aug, path, value); }
// int mgmt_aug_load(augeas *aug) { return aug_load_fn(aug); }
// int mgmt_aug_save(augeas *aug) { return aug_save_fn(aug); }
// void mgmt_aug_close(augeas *aug) { aug_close_fn(aug); }
// int mgmt_aug_error(augeas *aug) { return aug_error_fn(aug); }
// const char *mgmt_aug_error_message(augeas *aug) { return aug_error_message_fn(aug); }
// const char *mgmt_aug_error_minor_message(augeas *aug) { return aug_error_minor_message_fn(aug); }
// const char *mgmt_aug_error_details(augeas *aug) { return aug_error_details_fn(aug); }
import "C"

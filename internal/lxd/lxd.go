// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

// Ensure ensures that an LXD is available for the given username, and returns
// its address. If the LXD is not available, one is created from the given
// imageName.
func Ensure(username, imageName string) (string, error) {
	return "", nil
}

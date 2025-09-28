// Package updater downloads and applies updates from the server.
//
// It validates local files against checksums from a remote manifest, downloads
// required artifacts to a temporary directory, atomically applies updates, and
// starts the appropriate executable for the current role.
package updater

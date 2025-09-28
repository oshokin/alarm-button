// Package packager prepares the update manifest consumed by the updater.
//
// It computes checksums for platform-specific binaries, wires role-to-files
// mappings, and persists connection settings. The resulting YAML is uploaded
// to the update folder served to clients.
package packager

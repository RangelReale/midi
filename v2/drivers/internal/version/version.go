package version

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Version represents a version (e.g. a release version)
type Version struct {
	Major uint16
	Minor uint16
	Patch uint16
}

// String returns the string representation of the version (without a preceeding v
func (me Version) String() string {
	return fmt.Sprintf("%v.%v.%v", me.Major, me.Minor, me.Patch)
}

// Equals is true when the version exactly equals the given version
func (me Version) Equals(o Version) bool {
	return me.Major == o.Major && me.Minor == o.Minor && me.Patch == o.Patch
}

// EqualsMajor is true when the version equals the given version on the Major
func (me Version) EqualsMajor(o Version) bool {
	return me.Major == o.Major
}

// EqualsMinor is true when the version equals the given version on the Major and Minor
func (me Version) EqualsMinor(o Version) bool {
	return me.Major == o.Major && me.Minor == o.Minor
}

// Less returns true, if the Version is smaller than the given one.
func (me Version) Less(o Version) bool {
	if me.Major != o.Major {
		return me.Major < o.Major
	}

	if me.Minor != o.Minor {
		return me.Minor < o.Minor
	}

	return me.Patch < o.Patch
}

// Parse parses the version out of the given string. Valid strings are "v0.0.1" or "1.0" or "12" etc.
func Parse(v string) (*Version, error) {
	v = strings.TrimLeft(v, "v")
	nums := strings.Split(v, ".")
	var ns = make([]uint16, len(nums))

	var ver Version

	for i, n := range nums {
		nn, err := strconv.Atoi(n)
		if err != nil {
			return nil, err
		}
		if nn < 0 {
			return nil, fmt.Errorf("number must not be < 0")
		}
		ns[i] = uint16(nn)
	}

	switch len(ns) {
	case 1:
		if ns[0] == 0 {
			return nil, fmt.Errorf("invalid version %q", v)
		}
		ver.Major = ns[0]
	case 2:
		if (ns[0] + ns[1]) == 0 {
			return nil, fmt.Errorf("invalid version %q", v)
		}
		ver.Major = ns[0]
		ver.Minor = ns[1]
	case 3:
		if (ns[0] + ns[1] + ns[2]) == 0 {
			return nil, fmt.Errorf("invalid version %q", v)
		}
		ver.Major = ns[0]
		ver.Minor = ns[1]
		ver.Patch = ns[2]
	default:
		return nil, fmt.Errorf("invalid version string %q", v)
	}

	return &ver, nil

}

// Versions is a sortable slice of *Version
type Versions []*Version

// Less returns true, if the version of index a is less than the version of index b
func (me Versions) Less(a, b int) bool {
	return me[a].Less(*me[b])
}

// Len returns the number of *Version inside the slice
func (me Versions) Len() int {
	return len(me)
}

// Swap swaps the *Version of the index a with that of the index b
func (me Versions) Swap(a, b int) {
	me[a], me[b] = me[b], me[a]
}

// Sort sorts the slice and returns it
func (me Versions) Sort() Versions {
	sort.Sort(me)
	return me
}

// Last returns the last *Version of the slice.
func (me Versions) Last() *Version {
	if len(me) == 0 {
		return nil
	}

	return me[len(me)-1]
}

// First returns the first *Version of the slice.
func (me Versions) First() *Version {
	if len(me) == 0 {
		return nil
	}

	return me[0]
}

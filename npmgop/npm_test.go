package npmgop

import (
	"fmt"
	"path/filepath"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestFetchPackage(t *testing.T) {
	c := qt.New(t)

	npmp, err := FetchPackage("alpinejs")
	c.Assert(err, qt.IsNil)

	last := npmp.Versions[len(npmp.Versions)-1]

	c.Assert(last.Name, qt.Equals, "alpinejs")
	c.Assert(last.Version, qt.Equals, "v3.3.3")
	c.Assert(last.Dist, qt.DeepEquals, Dist{ShaSum: "966c94b6847f3d6840c5750e0b14caec82214e56", Tarball: "https://registry.npmjs.org/alpinejs/-/alpinejs-3.3.3.tgz"})
	c.Assert(last.Dependencies, qt.DeepEquals, Dependencies{
		{Name: "@vue/reactivity", VersionRange: "^3.0.2"},
	})

	c.Assert(npmp.DistTags.Latest, qt.Equals, "v3.3.3")

	name := fmt.Sprintf("%s-%s-%s.tgz", last.Name, last.Version, last.Dist.ShaSum)

	tarFilename := filepath.Join(tempDir, name)

	c.Assert(downloadTarball(last.Dist, tarFilename), qt.IsNil)
	rc, err := repackTarballAsZip(tarFilename, last)
	c.Assert(err, qt.IsNil)
	c.Assert(rc.Close(), qt.IsNil)
}

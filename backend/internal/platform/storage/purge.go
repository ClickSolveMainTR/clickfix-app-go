package storage

import (
	"context"
	"fmt"
)

// PurgeObject removes all retained object versions before deleting the live key.
// Version listing failures fail closed; otherwise erasure could leave old bytes.
func PurgeObject(ctx context.Context, d Driver, key string) error {
	if key == "" {
		return nil
	}
	for batch := 0; batch < 10000; batch++ {
		versions, e := d.ListVersions(ctx, key)
		if e != nil {
			return e
		}
		if len(versions) == 0 {
			return d.Delete(ctx, key)
		}
		deleted := false
		for _, v := range versions {
			if v.VersionID != "" {
				if e = d.DeleteVersion(ctx, key, v.VersionID); e != nil {
					return e
				}
				deleted = true
			}
		}
		if !deleted {
			return d.Delete(ctx, key)
		}
	}
	return fmt.Errorf("storage: version purge limit reached")
}

package database_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const migrationsDir = "../../db/migrations"

var migrationFileName = regexp.MustCompile(`^(\d{6})_([a-z0-9_]+)\.(up|down)\.sql$`)

// TestMigrationsHaveNoVersionCollisions guards the migration numbering contract: one version
// number must map to exactly one change set, and versions must be contiguous from 000001
// (golang-migrate stops at the first gap, so a missing number silently skips every later
// migration). It is the test that would have caught two branches claiming the same number.
func TestMigrationsHaveNoVersionCollisions(t *testing.T) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", migrationsDir, err)
	}

	namesByVersion := map[string]map[string]struct{}{}
	directionsByVersion := map[string]map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		match := migrationFileName.FindStringSubmatch(entry.Name())
		if match == nil {
			t.Errorf("migration %q does not match NNNNNN_name.(up|down).sql", entry.Name())
			continue
		}
		version, name, direction := match[1], match[2], match[3]
		if namesByVersion[version] == nil {
			namesByVersion[version] = map[string]struct{}{}
			directionsByVersion[version] = map[string]struct{}{}
		}
		namesByVersion[version][name] = struct{}{}
		directionsByVersion[version][direction] = struct{}{}
	}

	if len(namesByVersion) == 0 {
		t.Fatal("no migrations found")
	}

	versions := make([]string, 0, len(namesByVersion))
	for version := range namesByVersion {
		versions = append(versions, version)
	}
	sort.Strings(versions)

	for i, version := range versions {
		if names := namesByVersion[version]; len(names) > 1 {
			t.Errorf("migration version %s is used by multiple names: %v", version, sortedKeys(names))
		}
		if want := fmt.Sprintf("%06d", i+1); version != want {
			t.Fatalf("migration versions are not contiguous: position %d is %s, want %s", i+1, version, want)
			return
		}
	}
}

// TestPurchaseAwaitingIndexMigration pins the P5 index: the partial index must cover the
// customer/created_at ordering, be restricted to the two awaiting statuses, and be dropped by
// its own down migration.
func TestPurchaseAwaitingIndexMigration(t *testing.T) {
	for _, direction := range []string{"up", "down"} {
		path := filepath.Join(migrationsDir, "000037_add_purchase_awaiting_index."+direction+".sql")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected migration pair %q: %v", path, err)
		}
	}

	upBytes, err := os.ReadFile(filepath.Join(migrationsDir, "000037_add_purchase_awaiting_index.up.sql"))
	if err != nil {
		t.Fatalf("read up migration error = %v", err)
	}
	up := normalizeSQL(string(upBytes))

	for _, want := range []string{
		"CREATE INDEX IF NOT EXISTS idx_purchase_customer_awaiting",
		"ON purchase (customer_id, created_at DESC)",
		"WHERE status IN ('pending', 'new')",
	} {
		if !strings.Contains(up, want) {
			t.Errorf("up migration missing %q; got:\n%s", want, up)
		}
	}

	downBytes, err := os.ReadFile(filepath.Join(migrationsDir, "000037_add_purchase_awaiting_index.down.sql"))
	if err != nil {
		t.Fatalf("read down migration error = %v", err)
	}
	if down := normalizeSQL(string(downBytes)); !strings.Contains(down, "DROP INDEX IF EXISTS idx_purchase_customer_awaiting") {
		t.Errorf("down migration does not drop idx_purchase_customer_awaiting; got:\n%s", down)
	}
}

func normalizeSQL(sql string) string {
	lines := strings.Split(sql, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		kept = append(kept, strings.Join(strings.Fields(trimmed), " "))
	}
	return strings.Join(kept, " ")
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

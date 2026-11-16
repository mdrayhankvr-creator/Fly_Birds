package admin

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"time"
)

// VerifyResetBackup checks the archive that makes a reset recoverable. The
// expected digest must come from the newly created backup, never from the
// archive being checked.
func VerifyResetBackup(path, expectedSHA, school string, schemaVersion int) error {
	if len(expectedSHA) != 64 || strings.Trim(expectedSHA, "0123456789abcdef") != "" {
		return errors.New("backup SHA-256 must be 64 lowercase hexadecimal characters")
	}
	actual, err := fileSHA256(path)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}
	if actual != expectedSHA {
		return errors.New("backup SHA-256 does not match")
	}
	var archivedSchool string
	manifest, _, err := ReadArchive(path, func(name string, body []byte, _ *Manifest) error {
		if name != "tables/school_settings.csv" {
			return nil
		}
		records, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
		if err != nil || len(records) != 2 {
			return errors.New("backup school settings are missing or malformed")
		}
		for i, column := range records[0] {
			if column == "name_en" {
				archivedSchool = records[1][i]
				return nil
			}
		}
		return errors.New("backup has no school name")
	})
	if err != nil {
		return fmt.Errorf("verify backup contents: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(archivedSchool), strings.TrimSpace(school)) {
		return fmt.Errorf("backup is for school %q, expected %q", archivedSchool, school)
	}
	if manifest.SchemaVersion != schemaVersion {
		return fmt.Errorf("backup schema is %d, database schema is %d", manifest.SchemaVersion, schemaVersion)
	}
	created, err := time.Parse(time.RFC3339, manifest.CreatedAt)
	if err != nil || created.After(time.Now().Add(5*time.Minute)) || time.Since(created) > 2*time.Hour {
		return errors.New("backup must have been created within the last two hours")
	}
	return nil
}

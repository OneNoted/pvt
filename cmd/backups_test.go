package cmd

import "testing"

func TestBackupRetentionRejectsZeroAndNegativeValues(t *testing.T) {
	prev := backupsOlderThanDays
	t.Cleanup(func() { backupsOlderThanDays = prev })

	for _, value := range []int{0, -1} {
		backupsOlderThanDays = value
		if _, err := backupRetention(); err == nil {
			t.Fatalf("backupRetention() with %d days returned nil error", value)
		}
	}
}

func TestBackupRetentionRejectsUnreasonablyLargeValues(t *testing.T) {
	prev := backupsOlderThanDays
	t.Cleanup(func() { backupsOlderThanDays = prev })

	backupsOlderThanDays = maxBackupRetentionDays + 1
	if _, err := backupRetention(); err == nil {
		t.Fatal("backupRetention() returned nil error for excessive retention days")
	}
}

func TestBackupRetentionAcceptsPositiveValues(t *testing.T) {
	prev := backupsOlderThanDays
	t.Cleanup(func() { backupsOlderThanDays = prev })

	backupsOlderThanDays = 1
	if got, err := backupRetention(); err != nil || got == 0 {
		t.Fatalf("backupRetention() = %s, %v; want positive duration", got, err)
	}
}

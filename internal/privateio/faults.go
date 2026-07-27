package privateio

// FaultPoint names a durable-write boundary. Tests inject failures at these
// points to prove that only the old or the fully written new document can be
// authoritative after a crash.
type FaultPoint string

const (
	FaultBeforeBackupWrite    FaultPoint = "before_backup_write"
	FaultAfterBackupWrite     FaultPoint = "after_backup_write"
	FaultBeforeBackupSync     FaultPoint = "before_backup_sync"
	FaultAfterBackupSync      FaultPoint = "after_backup_sync"
	FaultBeforeBackupRename   FaultPoint = "before_backup_rename"
	FaultAfterBackupRename    FaultPoint = "after_backup_rename"
	FaultBeforeBackupDirSync  FaultPoint = "before_backup_dir_sync"
	FaultAfterBackupDirSync   FaultPoint = "after_backup_dir_sync"
	FaultBeforeCurrentWrite   FaultPoint = "before_current_write"
	FaultAfterCurrentWrite    FaultPoint = "after_current_write"
	FaultBeforeCurrentSync    FaultPoint = "before_current_sync"
	FaultAfterCurrentSync     FaultPoint = "after_current_sync"
	FaultBeforeCurrentRename  FaultPoint = "before_current_rename"
	FaultAfterCurrentRename   FaultPoint = "after_current_rename"
	FaultBeforeCurrentDirSync FaultPoint = "before_current_dir_sync"
	FaultAfterCurrentDirSync  FaultPoint = "after_current_dir_sync"
	FaultBeforeReread         FaultPoint = "before_reread"
	FaultAfterReread          FaultPoint = "after_reread"
)

// FaultInjector is nil in production. An injected error is converted to a
// value-free durable-write error instead of being reflected to a caller.
type FaultInjector func(FaultPoint) error

func (inject FaultInjector) at(point FaultPoint) error {
	if inject == nil {
		return nil
	}
	return inject(point)
}

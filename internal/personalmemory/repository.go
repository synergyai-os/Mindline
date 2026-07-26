package personalmemory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	libraryFileName = "library.json"
	backupFileName  = "library.backup.json"
	lockFileName    = "library.lock"
	contentDirName  = "content"
)

type RepositoryPort interface {
	Load() (Library, error)
	Import(CaptureBatch) (ImportReceipt, error)
	MergeEnrichment(EnrichmentBatch) (EnrichmentReceipt, error)
	LoadContent(ContentArtifactRef) (ExtractedContentArtifact, error)
}

type FileRepository struct {
	root        string
	libraryPath string
	backupPath  string
	lockPath    string
	contentDir  string
	now         func() time.Time
}

func NewFileRepository(root string, now func() time.Time) (*FileRepository, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("personal evidence root must be absolute")
	}
	if now == nil {
		now = time.Now
	}
	root = filepath.Clean(root)
	if err := prepareRepositoryRoot(root); err != nil {
		return nil, errors.New("personal evidence storage unavailable")
	}
	repository := &FileRepository{
		root:        filepath.Clean(root),
		libraryPath: filepath.Join(root, libraryFileName),
		backupPath:  filepath.Join(root, backupFileName),
		lockPath:    filepath.Join(root, lockFileName),
		contentDir:  filepath.Join(root, contentDirName),
		now:         now,
	}
	if err := privateio.ValidateContained(repository.root, repository.libraryPath, repository.backupPath, repository.lockPath, repository.contentDir); err != nil {
		return nil, errors.New("personal evidence storage unavailable")
	}
	return repository, nil
}

func prepareRepositoryRoot(root string) error {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		if err := os.Mkdir(root, privateio.DirMode); err != nil {
			return err
		}
		return privateio.ValidateContained(root, root)
	}
	if err != nil || !info.IsDir() || info.Mode().Perm() != privateio.DirMode {
		return errors.New("personal evidence root must be a dedicated owner-only directory")
	}
	return privateio.ValidateContained(root, root)
}

func (repository *FileRepository) Load() (Library, error) {
	var library Library
	err := privateio.ReadJSONStrictBounded(repository.root, repository.libraryPath, MaximumLibraryBytes, &library)
	if errors.Is(err, fs.ErrNotExist) {
		return EmptyLibrary(), nil
	}
	if err != nil || validateLibrary(library) != nil {
		return Library{}, errors.New("personal evidence library unavailable")
	}
	return library, nil
}

func (repository *FileRepository) Import(batch CaptureBatch) (ImportReceipt, error) {
	if err := validateCaptureBatch(batch); err != nil {
		return ImportReceipt{}, err
	}
	batchFingerprint := captureBatchFingerprint(batch)
	lock, err := privateio.AcquireAdvisoryLock(repository.root, repository.lockPath)
	if err != nil {
		return ImportReceipt{}, errors.New("personal evidence library busy")
	}
	defer lock.Close()
	library, err := repository.Load()
	if err != nil {
		return ImportReceipt{}, err
	}
	for _, existing := range library.Imports {
		if existing.BatchFingerprint == batchFingerprint {
			return ImportReceipt{
				BatchFingerprint: batchFingerprint,
				SourceIdentity:   batch.SourceIdentity,
				LowerInclusive:   batch.LowerInclusive,
				UpperInclusive:   batch.UpperInclusive,
				Watermark:        batch.Watermark,
				DeclaredRecords:  batch.DeclaredRecords,
				UnchangedRecords: len(batch.Records),
				TotalRecords:     len(library.Records),
				ImportedAt:       existing.ImportedAt,
			}, nil
		}
	}
	byKey := make(map[string]int, len(library.Records))
	for index, record := range library.Records {
		byKey[record.IdempotencyKey] = index
	}
	resourcesByID := make(map[string]bool, len(library.Resources))
	for _, resource := range library.Resources {
		resourcesByID[resource.ResourceID] = true
	}
	revisionsByID := make(map[string]bool, len(library.Revisions))
	for _, revision := range library.Revisions {
		revisionsByID[revision.RevisionID] = true
	}
	receipt := ImportReceipt{
		BatchFingerprint: batchFingerprint,
		SourceIdentity:   batch.SourceIdentity,
		LowerInclusive:   batch.LowerInclusive,
		UpperInclusive:   batch.UpperInclusive,
		Watermark:        batch.Watermark,
		DeclaredRecords:  batch.DeclaredRecords,
		ImportedAt:       repository.now().UTC().Format(time.RFC3339Nano),
	}
	for _, record := range batch.Records {
		for index, resourceID := range record.ResourceIDs {
			if resourcesByID[resourceID] {
				continue
			}
			library.Resources = append(library.Resources, placeholderResource(record.URLs[index]))
			resourcesByID[resourceID] = true
		}
		index, exists := byKey[record.IdempotencyKey]
		if !exists {
			library.Records = append(library.Records, record)
			byKey[record.IdempotencyKey] = len(library.Records) - 1
			receipt.InsertedRecords++
			continue
		}
		if library.Records[index].ContentHash == record.ContentHash {
			receipt.UnchangedRecords++
			continue
		}
		prior := library.Records[index]
		revisionID := stableRevisionID(prior)
		if !revisionsByID[revisionID] {
			library.Revisions = append(library.Revisions, CaptureRevision{
				RevisionID: revisionID, SupersededAt: receipt.ImportedAt, Record: prior,
			})
			revisionsByID[revisionID] = true
		}
		library.Records[index] = record
		receipt.UpdatedRecords++
	}
	receipt.TotalRecords = len(library.Records)
	library.Revision++
	library.Imports = append(library.Imports, receipt)
	library = sealLibrary(library)
	if err := repository.persistLibrary(library); err != nil {
		return ImportReceipt{}, err
	}
	return receipt, nil
}

func (repository *FileRepository) MergeEnrichment(batch EnrichmentBatch) (EnrichmentReceipt, error) {
	if batch.SchemaVersion != EnrichmentBatchSchemaVersion || len(batch.Resources)+len(batch.Contents) == 0 {
		return EnrichmentReceipt{}, errors.New("personal evidence enrichment is empty")
	}
	if len(batch.Resources)+len(batch.Contents) > MaximumResources {
		return EnrichmentReceipt{}, errors.New("personal evidence enrichment exceeds resource limit")
	}
	contentByURL := map[string]*ContentArtifactRef{}
	contentPayloads := map[string][]byte{}
	contentMissingnessByURL := map[string][]string{}
	contentAccessByURL := map[string]string{}
	secretContentURLs := map[string]bool{}
	for _, content := range batch.Contents {
		canonicalURL, reference, payload, contentMissingness, contentAccess, secretLike, err := prepareExtractedContent(content)
		if err != nil {
			return EnrichmentReceipt{}, err
		}
		if _, duplicate := contentByURL[canonicalURL]; duplicate || secretContentURLs[canonicalURL] {
			return EnrichmentReceipt{}, errors.New("personal evidence enrichment contains duplicate content")
		}
		if secretLike {
			secretContentURLs[canonicalURL] = true
			continue
		}
		contentByURL[canonicalURL] = reference
		contentMissingnessByURL[canonicalURL] = contentMissingness
		contentAccessByURL[canonicalURL] = contentAccess
		contentPayloads[reference.ArtifactID] = payload
	}
	seenInputs := map[string]bool{}
	resources := make([]ResourceContext, 0, len(batch.Resources)+len(batch.Contents))
	for _, input := range batch.Resources {
		var resource ResourceContext
		var err error
		if secretContentURLs[input.CanonicalURL] {
			resource = redactedResource(input.CanonicalURL)
		} else {
			resource, err = resourceFromImportedEvidence(input, contentByURL[input.CanonicalURL], contentMissingnessByURL[input.CanonicalURL], contentAccessByURL[input.CanonicalURL])
		}
		if err != nil {
			return EnrichmentReceipt{}, err
		}
		if seenInputs[resource.ResourceID] {
			return EnrichmentReceipt{}, errors.New("personal evidence enrichment contains duplicate resources")
		}
		seenInputs[resource.ResourceID] = true
		resources = append(resources, resource)
	}
	for canonicalURL, reference := range contentByURL {
		resourceID := stableResourceID(canonicalURL)
		if seenInputs[resourceID] {
			continue
		}
		resource := resourceFromContentOnly(canonicalURL, *reference, contentMissingnessByURL[canonicalURL], contentAccessByURL[canonicalURL])
		seenInputs[resourceID] = true
		resources = append(resources, resource)
	}
	for canonicalURL := range secretContentURLs {
		resourceID := stableResourceID(canonicalURL)
		if seenInputs[resourceID] {
			continue
		}
		seenInputs[resourceID] = true
		resources = append(resources, redactedResource(canonicalURL))
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].ResourceID < resources[j].ResourceID
	})
	inputFingerprint := fingerprintValue(resources)
	lock, err := privateio.AcquireAdvisoryLock(repository.root, repository.lockPath)
	if err != nil {
		return EnrichmentReceipt{}, errors.New("personal evidence library busy")
	}
	defer lock.Close()
	library, err := repository.Load()
	if err != nil {
		return EnrichmentReceipt{}, err
	}
	for _, existing := range library.EnrichmentImports {
		if existing.InputFingerprint == inputFingerprint {
			return EnrichmentReceipt{
				InputFingerprint:   inputFingerprint,
				DeclaredResources:  len(resources),
				UnchangedResources: len(resources),
				TotalResources:     len(library.Resources),
				ImportedAt:         existing.ImportedAt,
			}, nil
		}
	}
	if err := repository.pruneUnreferencedContent(library); err != nil {
		return EnrichmentReceipt{}, err
	}
	allowed := map[string]bool{}
	for _, record := range library.Records {
		for _, resourceID := range record.ResourceIDs {
			allowed[resourceID] = true
		}
	}
	for _, revision := range library.Revisions {
		for _, resourceID := range revision.Record.ResourceIDs {
			allowed[resourceID] = true
		}
	}
	resourceGraph := make([]ResourceContext, 0, len(library.Resources)+len(resources))
	resourceGraph = append(resourceGraph, library.Resources...)
	resourceGraph = append(resourceGraph, resources...)
	for changed := true; changed; {
		changed = false
		for _, resource := range resourceGraph {
			if !allowed[resource.ResourceID] {
				continue
			}
			for _, related := range resource.RelatedURLs {
				relatedID := stableResourceID(related.URL)
				if !allowed[relatedID] {
					allowed[relatedID] = true
					changed = true
				}
			}
		}
	}
	for _, resource := range resources {
		if !allowed[resource.ResourceID] {
			return EnrichmentReceipt{}, errors.New("personal evidence enrichment is not reachable from a retained capture")
		}
	}
	byID := make(map[string]int, len(library.Resources))
	for index, resource := range library.Resources {
		byID[resource.ResourceID] = index
	}
	resourceRevisionIDs := make(map[string]bool, len(library.ResourceRevisions))
	for _, revision := range library.ResourceRevisions {
		resourceRevisionIDs[revision.RevisionID] = true
	}
	receipt := EnrichmentReceipt{
		InputFingerprint:  inputFingerprint,
		DeclaredResources: len(resources),
		ImportedAt:        repository.now().UTC().Format(time.RFC3339Nano),
	}
	createdArtifacts := []ContentArtifactRef{}
	committed := false
	defer func() {
		if !committed {
			repository.removeContentArtifacts(createdArtifacts)
		}
	}()
	for _, resource := range resources {
		if resource.Content != nil {
			created, err := repository.writeContentArtifact(*resource.Content, contentPayloads[resource.Content.ArtifactID])
			if err != nil {
				return EnrichmentReceipt{}, err
			}
			if created {
				createdArtifacts = append(createdArtifacts, *resource.Content)
			}
		}
		index, exists := byID[resource.ResourceID]
		if !exists {
			library.Resources = append(library.Resources, resource)
			byID[resource.ResourceID] = len(library.Resources) - 1
			receipt.InsertedResources++
			continue
		}
		if library.Resources[index].ContentHash == resource.ContentHash {
			receipt.UnchangedResources++
			continue
		}
		prior := library.Resources[index]
		revisionID := stableResourceRevisionID(prior)
		if !resourceRevisionIDs[revisionID] {
			library.ResourceRevisions = append(library.ResourceRevisions, ResourceRevision{
				RevisionID: revisionID, SupersededAt: receipt.ImportedAt, Resource: prior,
			})
			resourceRevisionIDs[revisionID] = true
		}
		library.Resources[index] = resource
		receipt.UpdatedResources++
	}
	for _, resource := range resources {
		for _, related := range resource.RelatedURLs {
			resourceID := stableResourceID(related.URL)
			if _, exists := byID[resourceID]; exists {
				continue
			}
			placeholder := placeholderResource(related.URL)
			library.Resources = append(library.Resources, placeholder)
			byID[resourceID] = len(library.Resources) - 1
			receipt.InsertedResources++
		}
	}
	receipt.TotalResources = len(library.Resources)
	library.Revision++
	library.EnrichmentImports = append(library.EnrichmentImports, receipt)
	library = sealLibrary(library)
	if err := repository.persistLibrary(library); err != nil {
		return EnrichmentReceipt{}, err
	}
	committed = true
	return receipt, nil
}

func (repository *FileRepository) LoadContent(reference ContentArtifactRef) (ExtractedContentArtifact, error) {
	if err := validateContentReference(reference); err != nil {
		return ExtractedContentArtifact{}, err
	}
	path := filepath.Join(repository.contentDir, reference.ArtifactID+".txt")
	if err := privateio.ValidateContained(repository.root, repository.contentDir, path); err != nil {
		return ExtractedContentArtifact{}, errors.New("personal evidence content unavailable")
	}
	payload, err := privateio.ReadFileBounded(repository.contentDir, path, int64(MaximumExtractedContentBytes))
	if err != nil || len(payload) != reference.ByteLength || len([]rune(string(payload))) != reference.RuneCount {
		return ExtractedContentArtifact{}, errors.New("personal evidence content unavailable")
	}
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != reference.SHA256 {
		return ExtractedContentArtifact{}, errors.New("personal evidence content fingerprint mismatch")
	}
	return ExtractedContentArtifact{Reference: reference, Text: string(payload)}, nil
}

func (repository *FileRepository) writeContentArtifact(reference ContentArtifactRef, payload []byte) (bool, error) {
	if err := validateContentReference(reference); err != nil || len(payload) != reference.ByteLength {
		return false, errors.New("personal evidence content is invalid")
	}
	if err := privateio.PrepareDir(repository.contentDir); err != nil {
		return false, errors.New("personal evidence content storage unavailable")
	}
	path := filepath.Join(repository.contentDir, reference.ArtifactID+".txt")
	if err := privateio.ValidateContained(repository.root, repository.contentDir, path); err != nil {
		return false, errors.New("personal evidence content storage unavailable")
	}
	if existing, err := privateio.ReadFileBounded(repository.contentDir, path, int64(MaximumExtractedContentBytes)); err == nil {
		if !bytes.Equal(existing, payload) {
			return false, errors.New("personal evidence content identity collision")
		}
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, errors.New("personal evidence content storage unavailable")
	}
	usage, err := repository.contentUsageBytes()
	if err != nil || int64(len(payload)) > MaximumRepositoryContentBytes-usage {
		return false, errors.New("personal evidence content repository exceeds its storage budget")
	}
	if err := privateio.WriteFile(path, payload, true); err != nil {
		return false, errors.New("personal evidence content storage unavailable")
	}
	return true, nil
}

func (repository *FileRepository) contentUsageBytes() (int64, error) {
	if err := repository.validateContentDirectory(); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(repository.contentDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != privateio.FileMode {
			return 0, errors.New("personal evidence content storage unavailable")
		}
		if !validContentArtifactFileName(entry.Name()) || info.Size() < 1 ||
			info.Size() > int64(MaximumExtractedContentBytes) ||
			info.Size() > MaximumRepositoryContentBytes-total {
			return 0, errors.New("personal evidence content storage unavailable")
		}
		total += info.Size()
	}
	return total, nil
}

func (repository *FileRepository) pruneUnreferencedContent(library Library) error {
	if err := repository.validateContentDirectory(); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	entries, err := os.ReadDir(repository.contentDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("personal evidence content storage unavailable")
	}
	referenced := map[string]bool{}
	for _, resource := range library.Resources {
		if resource.Content != nil {
			referenced[resource.Content.ArtifactID+".txt"] = true
		}
	}
	for _, revision := range library.ResourceRevisions {
		if revision.Resource.Content != nil {
			referenced[revision.Resource.Content.ArtifactID+".txt"] = true
		}
	}
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != privateio.FileMode ||
			!validContentArtifactFileName(entry.Name()) {
			return errors.New("personal evidence content storage unavailable")
		}
		if referenced[entry.Name()] {
			continue
		}
		path := filepath.Join(repository.contentDir, entry.Name())
		if err := privateio.ValidateContained(repository.root, repository.contentDir, path); err != nil {
			return errors.New("personal evidence content storage unavailable")
		}
		if err := os.Remove(path); err != nil {
			return errors.New("personal evidence content storage unavailable")
		}
	}
	return nil
}

func (repository *FileRepository) validateContentDirectory() error {
	info, err := os.Lstat(repository.contentDir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != privateio.DirMode {
		return errors.New("personal evidence content storage unavailable")
	}
	return privateio.ValidateContained(repository.root, repository.contentDir)
}

func (repository *FileRepository) removeContentArtifacts(references []ContentArtifactRef) {
	for _, reference := range references {
		path := filepath.Join(repository.contentDir, reference.ArtifactID+".txt")
		if privateio.ValidateContained(repository.root, repository.contentDir, path) == nil {
			_ = os.Remove(path)
		}
	}
}

func validContentArtifactFileName(name string) bool {
	if !strings.HasPrefix(name, "content-") || !strings.HasSuffix(name, ".txt") {
		return false
	}
	digest := strings.TrimSuffix(strings.TrimPrefix(name, "content-"), ".txt")
	return validSHA256(digest)
}

func (repository *FileRepository) Status() (Status, error) {
	library, err := repository.Load()
	if err != nil {
		return Status{}, err
	}
	return Status{
		SchemaVersion:           library.SchemaVersion,
		Revision:                library.Revision,
		Fingerprint:             library.Fingerprint,
		RecordCount:             len(library.Records),
		ResourceCount:           len(library.Resources),
		HistoricalRevisionCount: len(library.Revisions),
		HistoricalResourceCount: len(library.ResourceRevisions),
		ImportCount:             len(library.Imports),
		EnrichmentImportCount:   len(library.EnrichmentImports),
		AuthorityClass:          AuthorityClass,
	}, nil
}

func validateCaptureBatch(batch CaptureBatch) error {
	if batch.SchemaVersion != CaptureBatchSchemaVersion || batch.SourceIdentity == "" ||
		batch.LowerInclusive == "" || batch.UpperInclusive == "" || batch.Watermark == "" ||
		batch.DeclaredRecords != len(batch.Records) || len(batch.Records) > MaximumRecords {
		return errors.New("invalid personal evidence capture batch")
	}
	batchText := []string{
		batch.SourceIdentity, batch.LowerInclusive, batch.UpperInclusive, batch.Watermark,
	}
	if importedEvidenceContainsSecret(batchText...) ||
		containsUnsafeURL(strings.Join(batchText, "\n")) {
		return errors.New("personal evidence capture batch contains unsafe material")
	}
	keys := map[string]bool{}
	for _, record := range batch.Records {
		if err := validateRecord(record); err != nil || keys[record.IdempotencyKey] {
			return errors.New("invalid personal evidence capture batch")
		}
		keys[record.IdempotencyKey] = true
	}
	return nil
}

func captureBatchFingerprint(batch CaptureBatch) string {
	type recordProof struct {
		RecordID    string `json:"record_id"`
		ContentHash string `json:"content_hash"`
	}
	proofs := make([]recordProof, 0, len(batch.Records))
	for _, record := range batch.Records {
		proofs = append(proofs, recordProof{RecordID: record.RecordID, ContentHash: record.ContentHash})
	}
	return fingerprintValue(struct {
		SchemaVersion, SourceIdentity, LowerInclusive, UpperInclusive, Watermark string
		DeclaredRecords                                                          int
		Records                                                                  []recordProof
	}{
		batch.SchemaVersion, batch.SourceIdentity, batch.LowerInclusive,
		batch.UpperInclusive, batch.Watermark, batch.DeclaredRecords, proofs,
	})
}

func (repository *FileRepository) persistLibrary(library Library) error {
	next, err := privateio.CanonicalJSONBytes(library)
	if err != nil {
		return errors.New("personal evidence library unavailable")
	}
	var prior []byte
	if _, err := os.Lstat(repository.libraryPath); err == nil {
		prior, err = privateio.ReadFileBounded(repository.root, repository.libraryPath, MaximumLibraryBytes)
		if err != nil {
			return errors.New("personal evidence library unavailable")
		}
	}
	validate := func(data []byte) error {
		var persisted Library
		if err := privateio.DecodeJSONStrict(data, &persisted); err != nil {
			return err
		}
		return validateLibrary(persisted)
	}
	if err := privateio.AtomicReplaceWithBackup(repository.root, repository.libraryPath, repository.backupPath, next, prior, MaximumLibraryBytes, validate, nil); err != nil {
		return errors.New("personal evidence library unavailable")
	}
	return nil
}

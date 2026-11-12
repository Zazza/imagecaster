package preset

import "io/fs"

func installSourcesForTest(db *DB, sources []PresetSource) (int, error) {
	models, _, err := SourceDependencies(sources)
	if err != nil {
		return 0, err
	}
	return db.SyncModelPresets(sources, append(models, ""), nil)
}

func installRawForTest(db *DB, raw []byte) (int, error) {
	return installSourcesForTest(db, []PresetSource{{File: "test.json", Raw: raw}})
}

func seedAllForTest(db *DB, fsys fs.FS) error {
	sources, err := BundledSources(fsys)
	if err != nil {
		return err
	}
	_, err = installSourcesForTest(db, sources)
	return err
}

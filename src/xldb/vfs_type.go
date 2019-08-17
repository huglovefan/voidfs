package xldb

type VfsType string

// used to distinguish symlink targets from dirs/files
const XLDB_DIR = VfsType("!D!")
const XLDB_FILE = VfsType("!F!")

func (vtype VfsType) Ok() bool {
	return vtype != ""
}

func (vtype VfsType) IsDir() bool {
	return vtype == XLDB_DIR
}

func (vtype VfsType) IsFile() bool {
	return vtype == XLDB_FILE
}

func (vtype VfsType) IsLink() bool {
	return vtype.Ok() && !vtype.IsDir() && !vtype.IsFile()
}

func (vtype VfsType) GetTarget() string {
	return string(vtype)
}

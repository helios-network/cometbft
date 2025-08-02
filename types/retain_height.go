package types

const (
	FlagArchive int64 = 1 << 62 // bit 62
	FlagMask    int64 = FlagArchive
)

func SetRetainHeightArchiveFlag(x int64) int64 {
	return x | FlagArchive
}

func IsRetainHeightArchive(x int64) bool {
	return (x & FlagArchive) != 0
}

func ClearRetainHeightArchiveFlag(x int64) int64 {
	return x &^ FlagArchive
}

func GetRetainHeightWithoutFlags(x int64) int64 {
	return x &^ FlagMask
}

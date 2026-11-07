package version

// Version is the Windows client/service/MSI product version.
const Version = "2.1.3"

func Label() string {
	return "v" + Version
}

package brand

import "fmt"

const HeaderRule = "────────────────────────────────────────────────────────"

func VersionedName(version string) string {
	name := Name
	if version != "" {
		name += " " + version
	}
	return name
}

func HeaderLine(mascot string, text string) string {
	return fmt.Sprintf("%-8s     %s", mascot, text)
}

func BadgeHeaderLine(mascot string, text string) string {
	return fmt.Sprintf("%s %s", mascot, text)
}

func MascotFrame(text string, face string) string {
	return fmt.Sprintf(" /\\_/\\  %s\n( %s )", text, face)
}

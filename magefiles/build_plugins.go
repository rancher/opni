package main

import "os"

func BuildPlugin(name string) error {
	// create bin/plugins if it doesn't exist
	if _, err := os.Stat("bin/plugins"); os.IsNotExist(err) {
		if err := os.Mkdir("bin/plugins", 0755); err != nil {
			return err
		}
	}
	return goBuild("./plugins/"+name, "-o", "./bin/plugins/plugin_"+name)
}

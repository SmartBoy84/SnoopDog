package main

// to be done:
// notification permission (check gist)
// local network permissions (check gist)

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"howett.net/plist"
)

var (
	perms  = strings.Split("kTCCServiceExposureNotification kTCCServiceFallDetection kTCCServiceGameCenterFriends kTCCServiceSensorKitBedSensingWriting kTCCServiceUserTracking kTCCServiceSiri kTCCServiceSpeechRecognition kTCCServiceAddressBook kTCCServiceBluetoothAlways kTCCServiceBluetoothPeripheral kTCCServiceBluetoothWhileInUse kTCCServiceCalendar kTCCServiceCalls kTCCServiceCamera kTCCServiceContactsFull kTCCServiceContactsLimited kTCCServiceMediaLibrary kTCCServiceMicrophone kTCCServiceMotion kTCCServicePhotosAdd kTCCServiceReminders kTCCServiceWillow", " ")
	dbPath = "/private/var/mobile/Library/TCC/TCC.db"

	clientsPlist = "/private/var/root/Library/Caches/locationd/clients.plist"
)

func restart_serivce(name string) error {
	cmd := exec.Command("launchctl", "load", fmt.Sprintf("/System/Library/LaunchDaemons/com.apple.%s.plist", name))
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func backup_file(sourcePath string) error {

	sourceFile, err := os.OpenFile(sourcePath, os.O_RDONLY, os.ModeAppend)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	backupPath := filepath.Join(filepath.Dir(sourcePath), filepath.Base(sourcePath)+".bak")

	fmt.Printf("Backing up %s to %s\n", sourcePath, backupPath)

	backupFile, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	defer backupFile.Close()

	_, err = io.Copy(backupFile, sourceFile)
	if err != nil {
		return err
	}

	// preserve permission
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	sourceMode := sourceInfo.Mode()

	err = os.Chmod(backupPath, sourceMode)
	if err != nil {
		return err
	}

	return nil
}

// general permissions
func patch_tcc(bundleName string, add bool) error {

	fmt.Println("Patching TCC.db")

	if err := backup_file(dbPath); err != nil {
		return err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	time := fmt.Sprint(time.Now().Unix())

	query := ""
	for _, perm := range perms {
		if add {
			query += fmt.Sprintf("INSERT INTO access VALUES('%s', '%s', 0, 2, 2, 1, NULL, NULL, NULL, 'UNUSED', NULL, 0, '%s');", perm, bundleName, time)
		} else {
			query += fmt.Sprintf("DELETE FROM access WHERE service = '%s' AND client = '%s';", perm, bundleName)
		}
	}

	if _, err := db.Exec(query); err != nil {
		return err
	}

	if err := restart_serivce("tccd"); err != nil {
		return err
	}

	return nil
}

// location permissions
func patch_location(bundleName string, executablePath string, add bool) error {

	fmt.Println("Patching locationd's clients.plist to grant location perms")

	if err := backup_file(clientsPlist); err != nil {
		return err
	}

	plistFile, err := os.Open(clientsPlist)
	if err != nil {
		return err
	}
	defer plistFile.Close()

	var data interface{}
	decoder := plist.NewDecoder(plistFile)
	err = decoder.Decode(&data)
	if err != nil {
		return err
	}

	plistData, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid plist file format")
	}

	if add {
		plistData[bundleName] = map[string]interface{}{
			"Authorization":              2,
			"BundleId":                   bundleName,
			"Executable":                 executablePath,
			"Registered":                 executablePath,
			"SupportedAuthorizationMask": 7,
			"Whitelisted":                false,
		}
	} else {
		delete(plistData, bundleName)
	}

	output, err := plist.Marshal(plistData, plist.AutomaticFormat)
	if err != nil {
		return err
	}

	if err := os.WriteFile(clientsPlist, output, 0644); err != nil {
		return err
	}

	if err := restart_serivce("locationd"); err != nil {
		return err
	}

	return nil
}

func main() {

	if len(os.Args) != 4 || (os.Args[1] != "add" && os.Args[1] != "remove") || (len(strings.Split(os.Args[2], ".")) != 3) {
		fmt.Print("{add/remove} {x.y.z} {/usr/bin/ls}")
		os.Exit(-1)
	}

	operation := os.Args[1] == "add"
	bundleName := os.Args[2]
	executablePath := os.Args[3]

	fmt.Printf("Getting permissions for %s...\n", bundleName)

	if err := patch_tcc(bundleName, operation); err != nil {
		panic(err)
	}

	if err := patch_location(bundleName, executablePath, operation); err != nil {
		panic(err)
	}
}

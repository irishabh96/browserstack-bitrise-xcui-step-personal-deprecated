package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func main() {
	// Exit after 2hours 30 mins
	time.AfterFunc(150*time.Minute, func() { failf("Session Timed Out") })

	username := os.Getenv("browserstack_username")
	access_key := os.Getenv("browserstack_accesskey")
	ios_app := os.Getenv("app_ipa_path")
	bundle_path := os.Getenv("xcui_test_suite")

	if username == "" || access_key == "" {
		failf(UPLOAD_APP_ERROR, "invalid credentials")
	}

	if ios_app == "" || bundle_path == "" {
		failf(FILE_NOT_AVAILABLE_ERROR)
	}

	out1, err1 := exec.Command("cp", "-r", bundle_path+"/Debug-iphoneos/"+"Tests iOS-Runner.app", ".").Output()
	if err1 != nil {
		log.Print("err1")
		fmt.Println(err1.Error())
	}

	fmt.Println("Done", out1)

	out, err := exec.Command("zip", "-r", "-D", "ideaz.zip", "Tests iOS-Runner.app").Output()
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Printf("The date is %s\n", out)
	fmt.Println("Done")
	test_runner_app := "ideaz.zip"

	log.Print("Starting the build on BrowserStack App Automate")

	upload_app, err := upload(ios_app, APP_UPLOAD_ENDPOINT, username, access_key)

	if err != nil {
		failf(err.Error())
	}

	upload_app_parsed_response := jsonParse(upload_app)

	log.Print(upload_app_parsed_response)

	if upload_app_parsed_response["app_url"] == "" {
		log.Print("ehre")
		failf(err.Error())
	}

	log.Print("heree", upload_app_parsed_response["app_url"])

	app_url := upload_app_parsed_response["app_url"].(string)

	upload_test_suite, err := upload(test_runner_app, TEST_SUITE_UPLOAD_ENDPOINT, username, access_key)

	if err != nil {
		failf(err.Error())
	}

	test_suite_upload_parsed_response := jsonParse(upload_test_suite)

	if test_suite_upload_parsed_response["test_suite_url"] == "" {
		failf(err.Error())
	}

	test_suite_url := test_suite_upload_parsed_response["test_suite_url"].(string)

	build_response, err := build(app_url, test_suite_url, username, access_key)

	if err != nil {
		failf(err.Error())
	}

	build_parsed_response := jsonParse(build_response)

	if build_parsed_response["message"] != "Success" {
		failf(BUILD_FAILED_ERROR, build_parsed_response["message"])
	}

	log.Print("Successfully started the build")

	check_build_status, _ := strconv.ParseBool(os.Getenv("check_build_status"))

	build_status := ""

	build_id := build_parsed_response["build_id"].(string)

	log.Println("Waiting for results")

	log.Printf("Build is running (BrowserStack build id %s)", build_id)

	build_status, err = checkBuildStatus(build_id, username, access_key, check_build_status)

	if err != nil {
		failf(err.Error())
	}

	cmd_log_build_id, err_build_id := exec.Command("bitrise", "envman", "add", "--key", "BROWSERSTACK_BUILD_URL", "--value", APP_AUTOMATE_BUILD_DASHBOARD_URL+build_parsed_response["build_id"].(string)).CombinedOutput()
	cmd_log_build_status, err_build_status := exec.Command("bitrise", "envman", "add", "--key", "BROWSERSTACK_BUILD_STATUS", "--value", build_status).CombinedOutput()

	if err_build_id != nil {
		fmt.Printf("Failed to expose output with envman, error: %#v | output: %s", err, cmd_log_build_id)
		os.Exit(1)
	}

	if err_build_status != nil {
		fmt.Printf("Failed to expose output with envman, error: %#v | output: %s", err, cmd_log_build_status)
		os.Exit(1)
	}

	os.Exit(0)
}

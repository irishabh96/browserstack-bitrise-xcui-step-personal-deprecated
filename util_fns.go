package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
)

func getDevices() ([]string, error) {
	var devices []string

	devices_input := os.Getenv("devices_list")

	if devices_input == "" {
		return devices, errors.New(fmt.Sprintf(BUILD_FAILED_ERROR, "invalid device format"))
	}

	scanner := bufio.NewScanner(strings.NewReader(devices_input))

	for scanner.Scan() {
		device := scanner.Text()
		device = strings.TrimSpace(device)

		if device == "" {
			continue
		}

		devices = append(devices, device)
	}

	return devices, nil
}

// any other capability which we're not taking from pre-defined inputs can be passed in api_params
func appendExtraCapabilities(payload string) []byte {
	out := map[string]interface{}{}

	json.Unmarshal([]byte(payload), &out)

	scanner := bufio.NewScanner(strings.NewReader(os.Getenv("api_params")))
	for scanner.Scan() {
		test_sharding := scanner.Text()

		test_sharding = strings.TrimSpace(test_sharding)

		if test_sharding == "" {
			continue
		}

		test_values := strings.Split(test_sharding, "=")

		key := test_values[0]

		out[key] = test_values[1]
	}

	outputJSON, _ := json.Marshal(out)

	return outputJSON
}

func getTestFilters(payload *BrowserStackPayload) {
	scanner := bufio.NewScanner(strings.NewReader(os.Getenv("filter_test")))
	for scanner.Scan() {
		test_filters := scanner.Text()

		test_filters = strings.TrimSpace(test_filters)

		if test_filters == "" {
			continue
		}

		test_values := strings.Split(test_filters, ",")

		for i := 0; i < len(test_values); i++ {
			test_value := strings.Split(test_values[i], " ")
			switch test_value[0] {
			case "skip-testing":
				*&payload.SkipTesting = append(*&payload.SkipTesting, test_value[1])
			case "only-testing":
				*&payload.OnlyTesting = append(*&payload.OnlyTesting, test_value[1])
			}
		}
	}
}

// this util only picks data from env and map it to the struct
func createBuildPayload() BrowserStackPayload {
	instrumentation_logs, _ := strconv.ParseBool(os.Getenv("instrumentation_logs"))
	network_logs, _ := strconv.ParseBool(os.Getenv("network_logs"))
	device_logs, _ := strconv.ParseBool(os.Getenv("device_logs"))
	debug_screenshots, _ := strconv.ParseBool(os.Getenv("debug_screenshots"))
	video_recording, _ := strconv.ParseBool(os.Getenv("video_recording"))
	use_local, _ := strconv.ParseBool(os.Getenv("use_local"))
	use_dynamic_tests, _ := strconv.ParseBool(os.Getenv("use_dynamic_tests"))

	sharding_data := TestSharding{}
	if os.Getenv("use_test_sharding") != "" {
		err := json.Unmarshal([]byte(os.Getenv("use_test_sharding")), &sharding_data)

		if err != nil {
			fmt.Println(err.Error())
		}
	}

	payload := BrowserStackPayload{
		InstrumentationLogs: instrumentation_logs,
		NetworkLogs:         network_logs,
		DeviceLogs:          device_logs,
		DebugScreenshots:    debug_screenshots,
		VideoRecording:      video_recording,
		DynamicTests:        use_dynamic_tests,
		Project:             os.Getenv("project"),
		ProjectNotifyURL:    os.Getenv("project_notify_url"),
		UseLocal:            use_local,
	}

	getTestFilters(&payload)

	if len(sharding_data.Mapping) != 0 && sharding_data.NumberOfShards != 0 {
		payload.UseTestSharding = sharding_data
	}

	payload.Devices, _ = getDevices()

	return payload
}

func failf(format string, args ...interface{}) {
	log.Fatalf(format, args...)
	os.Exit(1)
}

// this works as a goroutine which will run in background
// on a different thread without effecting any other code
func setInterval(someFunc func(), milliseconds int, async bool) chan bool {
	// How often to fire the passed in function
	// in milliseconds
	interval := time.Duration(milliseconds) * time.Millisecond

	// Setup the ticket and the channel to signal
	// the ending of the interval
	ticker := time.NewTicker(interval)
	clear := make(chan bool)

	// Put the selection in a go routine
	// so that the for loop is none blocking
	go func() {
		for {
			select {
			case <-ticker.C:
				if async {
					// This won't block
					go someFunc()
				} else {
					// This will block
					someFunc()
				}
			case <-clear:
				ticker.Stop()
			}
		}
	}()

	// We return the channel so we can pass in
	// a value to it to clear the interval
	return clear
}

func jsonParse(base64String string) map[string]interface{} {
	parsed_json := make(map[string]interface{})

	err := json.Unmarshal([]byte(base64String), &parsed_json)

	if err != nil {
		failf("Unable to parse app_upload API response: %s", err)
	}

	return parsed_json
}

// this function only print data to the console.
func printBuildStatus(build_details map[string]interface{}) {
	log.Println("Build finished")
	log.Println("Test results summary:")

	devices := build_details["devices"].([]interface{})
	build_id := build_details["id"]
	data := [][]string{}

	if len(devices) == 1 {
		sessions := devices[0].(map[string]interface{})["sessions"].([]interface{})[0].(map[string]interface{})

		session_status := sessions["status"].(string)
		session_test_cases := sessions["testcases"].(map[string]interface{})
		session_test_status := session_test_cases["status"].(map[string]interface{})

		total_test := session_test_cases["count"]
		passed_test := session_test_status["passed"]
		device_name := devices[0].(map[string]interface{})["device"].(string)

		if session_status == "passed" {
			result := fmt.Sprintf("PASSED (%v/%v passed)", passed_test, total_test)
			data = append(data, []string{build_id.(string), device_name, result})
		}

		if session_status == "failed" || session_status == "error" {
			result := fmt.Sprintf("FAILED (%v/%v passed)", passed_test, total_test)
			data = append(data, []string{build_id.(string), device_name, result})
		}
	} else {
		for i := 0; i < len(devices); i++ {
			sessions := devices[i].(map[string]interface{})["sessions"].([]interface{})[0].(map[string]interface{})

			session_status := sessions["status"].(string)
			session_test_cases := sessions["testcases"].(map[string]interface{})
			session_test_status := session_test_cases["status"].(map[string]interface{})

			total_test := session_test_cases["count"]
			passed_test := session_test_status["passed"]
			device_name := devices[i].(map[string]interface{})["device"].(string)

			if session_status == "passed" {
				result := fmt.Sprintf("PASSED (%v/%v passed)", passed_test, total_test)
				data = append(data, []string{build_id.(string), device_name, result})
			}

			if session_status == "failed" || session_status == "error" {
				result := fmt.Sprintf("FAILED (%v/%v passed)", passed_test, total_test)
				data = append(data, []string{build_id.(string), device_name, result})
			}
		}
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Build Id", "Devices", "Status"})

	for _, v := range data {
		table.Append(v)
	}

	table.Render()
}

func locateTestRunnerFileAndZip(test_suite_location string) error {
	split_test_suite_path := strings.Split(test_suite_location, "/")
	get_file_name := split_test_suite_path[len(split_test_suite_path)-1]

	test_runner_app_path := ""

	check_file_extension := strings.Split(get_file_name, ".")

	// Checking 2 conditions here
	// 1. test_suite_location - is this a the runner app
	// 2. test_suite_location - if this is a directory, does runner app exists in this directory.
	if len(check_file_extension) > 0 && check_file_extension[len(check_file_extension)-1] == "app" {
		test_runner_app_path = test_suite_location
	} else if strings.Contains(get_file_name, "test_bundle") {
		// if test_suite_location is a directory instead of the file, then check if runner app exits
		if _, err := os.Stat(test_suite_location + TEST_RUNNER_RELATIVE_PATH_BITRISE); errors.Is(err, os.ErrNotExist) {
			return errors.New(RUNNER_APP_NOT_FOUND)
		} else {
			test_runner_app_path = test_suite_location + TEST_RUNNER_RELATIVE_PATH_BITRISE
		}
	} else {
		return errors.New(RUNNER_APP_NOT_FOUND)
	}

	_, err := exec.Command("cp", "-r", test_runner_app_path, ".").Output()
	if err != nil {
		return errors.New(fmt.Sprintf(FILE_ZIP_ERROR, err))
	}

	_, zipping_err := exec.Command("zip", "-r", "-D", TEST_RUNNER_ZIP_FILE_NAME, "Tests iOS-Runner.app").Output()
	if zipping_err != nil {
		return errors.New(fmt.Sprintf(FILE_ZIP_ERROR, zipping_err))
	}

	return nil
}

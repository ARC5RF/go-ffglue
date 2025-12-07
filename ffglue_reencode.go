package ffglue

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ARC5RF/go-blame"
)

type ReEncodeOptions struct {
	Input    string
	Codec    string
	CRF      string
	Preset   string
	Extra    string
	Throttle string
}

type re_encode_task struct {
	tracker    Int64Tracker
	args       []string
	cmd        *exec.Cmd
	last_error string
}

func (task *re_encode_task) on_stdout(data []byte) (int, error) {
	data_str := string(data)
	lines := strings.Split(data_str, "\n")
	// fmt.Println("data_str", len(data_str), data_str)

	var val string
	for _, line := range lines {
		parts := strings.Split(line, "=")
		if len(parts) < 2 {
			continue
		}
		key := parts[0]
		if key == "out_time_ms" {
			val = parts[1]
			break
		}
	}
	if len(val) == 0 || val == "N/A" {
		return len(data), nil
	}

	time_micro, p_err := blame.O1(strconv.ParseInt(val, 10, 64))
	if p_err != nil {
		return 0, p_err
	}
	time_mili := time_micro / 1000

	task.tracker.SetValue(time_mili)

	return len(data), nil
}

func (task *re_encode_task) on_stderr(data []byte) (int, error) {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(data)), "\r\n", "\n"), "\n")
	task.last_error = lines[len(lines)-1]

	return len(data), nil
}

func (task *re_encode_task) start(input, throttle string, dur time.Duration) error {
	ffmpeg_args := append([]string{ffmpeg_filepath}, ffmpeg_progress_header...)
	ffmpeg_args = append(ffmpeg_args, task.args...)
	everything := append([]string{"-f", "-l", throttle, "--"}, ffmpeg_args...)

	fmt.Println(cpulimit_filepath, strings.Join(everything, " "))
	task.cmd = exec.Command(cpulimit_filepath, everything...)

	task.cmd.Stdout = &write_trap{task.on_stdout}
	task.cmd.Stderr = &write_trap{task.on_stderr}
	if start_err := task.cmd.Start(); start_err != nil {
		return start_err
	}

	task.tracker.Start(task.cmd.Process)
	task.tracker.SetTitle(input)
	task.tracker.SetTotal(int64(dur / time.Millisecond))

	return task.tracker.Finish(task.cmd.Wait())
}

func ReEncode(tracker Int64Tracker, options ReEncodeOptions, output string) error {
	dur, dur_err := blame.O1(VideoDuration(options.Input))
	if dur_err != nil {
		return dur_err
	}

	if options.CRF == "" {
		options.CRF = "24"
	}
	if options.Throttle == "" {
		options.Throttle = "100"
	}

	to_run := []string{"-i", options.Input, "-c:v", options.Codec, "-crf", options.CRF, "-preset", options.Preset, "-c:a", "copy", "-map", "0"}
	if len(options.Extra) > 1 {
		to_run = append(to_run, strings.Split(options.Extra, " ")...)
	}
	to_run = append(to_run, output)

	task := &re_encode_task{tracker: tracker, args: to_run}
	return blame.O0(task.start(options.Input, options.Throttle, dur))
}

var i_nope = errors.New("no input")

func i_flag(inputs []string) (string, error) {
	for i, input := range inputs {
		if input == "-i" {
			if i+1 > len(input)-1 {
				return "", i_nope
			}
			return inputs[i+1], nil
		}
	}
	return "", i_nope
}

func ReEncodeArgv(tracker Int64Tracker, throttle string, args ...string) error {
	input, input_err := blame.O1(i_flag(args))
	if input_err != nil {
		return input_err
	}
	dur, dur_err := blame.O1(VideoDuration(input))
	if dur_err != nil {
		return dur_err
	}

	task := &re_encode_task{tracker: tracker, args: args}
	return blame.O0(task.start(input, throttle, dur))
}

package ffglue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ARC5RF/go-blame"
	"github.com/ARC5RF/go-nanolex"
	"github.com/ARC5RF/go-which"
)

type write_trap struct{ callback func([]byte) (int, error) }

func (trap *write_trap) Write(data []byte) (int, error) { return trap.callback(data) }

var ffmpeg_filepath = which.MustFirst("ffmpeg")
var ffprobe_filepath = which.MustFirst("ffprobe")
var cpulimit_filepath = which.MustFirst("cpulimit")
var ffprobe_duration_fallback_args = []string{"-loglevel", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time", "-of", "csv=print_section=0"}
var ffprobe_duration_args = []string{"-v", "quiet", "-of", "default=noprint_wrappers=1:nokey=1", "-show_entries", "format=duration"}
var ffmpeg_progress_header = []string{"-y", "-loglevel", "error", "-progress", "pipe:1"}

func fallback_duration(to_probe string) (time.Duration, error) {
	prope_args := append([]string{}, ffprobe_duration_fallback_args...)
	prope_args = append(prope_args, to_probe)
	cmd := exec.Command("sh", "-c", ffprobe_filepath+" "+strings.Join(prope_args, " "))
	fmt.Println(cmd.Args[2])
	// var o bytes.Buffer
	var output time.Duration
	cmd.Stdout = &write_trap{func(b []byte) (int, error) {
		b_s := strings.TrimSpace(string(b))
		dur_f_sec, parse_dur_err := blame.O1(strconv.ParseFloat(b_s, 64))
		if parse_dur_err != nil {
			return len(b), nil
		}
		output = time.Duration(float64(time.Second) * dur_f_sec)
		return len(b), nil
	}}
	var e bytes.Buffer
	cmd.Stderr = &e

	run_res := blame.O0(cmd.Run())
	if run_res != nil {
		return -1, run_res.WithAdditionalContext("fallback", output.String(), e.String())
	}

	return output, nil
}

func VideoDuration(to_probe string) (time.Duration, error) {
	prope_args := append([]string{"-i", to_probe}, ffprobe_duration_args...)
	fmt.Println(ffprobe_filepath, strings.Join(prope_args, " "))
	cmd := exec.Command(ffprobe_filepath, prope_args...)
	var o bytes.Buffer
	cmd.Stdout = &o
	var e bytes.Buffer
	cmd.Stderr = &e
	// cmd.Stderr = &write_trap{pt.on_stderr}

	run_res := blame.O0(cmd.Run())
	if run_res != nil {
		return -1, run_res.WithAdditionalContext(e.String())
	}

	data_str := o.String()
	// if len(data_str) <= 7 {
	// 	return -1, blame.O0(errors.New("malformed output from ffprobe")).WithAdditionalContext(data_str, strings.Join(prope_args, " "))
	// }

	chunk := strings.TrimSpace(data_str)
	if chunk == "N/A" {
		return fallback_duration(to_probe)
	}
	dur_f_sec, parse_dur_err := blame.O1(strconv.ParseFloat(chunk, 64))
	if parse_dur_err != nil {
		return -1, parse_dur_err.WithAdditionalContext(ffprobe_filepath + " " + strings.Join(prope_args, " "))
	}

	return time.Duration(float64(time.Second) * dur_f_sec), nil
}

type Dimensions struct {
	W int
	H int
}

func Thumb(input string, timestamp time.Duration, output string, dim Dimensions) error {
	dur_s := fmt.Sprintf("%02d:%02d:%02d.000", int(timestamp.Hours()), int(timestamp.Minutes())%60, int(timestamp.Seconds())%60)
	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", dim.W, dim.H)
	prope_args := []string{"-ss", dur_s, "-i", input, "-vf", vf, "-vframes", "1", output}
	// fmt.Println(ffmpeg_filepath, strings.Join(prope_args, " "))
	cmd := exec.Command(ffmpeg_filepath, prope_args...)
	var o bytes.Buffer
	cmd.Stdout = &o
	var e bytes.Buffer
	cmd.Stderr = &e

	run_res := blame.O0(cmd.Run())
	if run_res != nil {
		return run_res.WithAdditionalContext(e.String())
	}
	// data_str := o.String()
	// fmt.Println(data_str)

	return nil
}

type enum_codec_type struct {
	letter string
	human  string
}

func (t enum_codec_type) String() string {
	return t.human
}

var VideoCodec = &enum_codec_type{"V", "Video Codec"}
var AudioCodec = &enum_codec_type{"A", "Audio Codec"}
var SubtitleCodec = &enum_codec_type{"S", "Subtitle Codec"}
var DataCodec = &enum_codec_type{"D", "Data Codec"}
var AttachmentCodec = &enum_codec_type{"A", "Attachment Codec"}

// both Lossy and Losslss can be true simultaneously because of individual codec config options
type CodecFlags struct {
	DecodingSupported bool
	EncodingSupported bool
	Type              *enum_codec_type
	IntraFrameOnly    bool
	Lossy             bool
	Losslss           bool
}

type codec_flags_json struct {
	DecodingSupported bool
	EncodingSupported bool
	Type              string
	IntraFrameOnly    bool
	Lossy             bool
	Losslss           bool
}

func (c CodecFlags) MarshalJSON() ([]byte, error) {
	return blame.O1(json.Marshal(codec_flags_json{
		DecodingSupported: c.DecodingSupported,
		EncodingSupported: c.EncodingSupported,
		Type:              c.Type.String(),
		IntraFrameOnly:    c.IntraFrameOnly,
		Lossy:             c.Lossy,
		Losslss:           c.Losslss,
	}))
}

func (c CodecFlags) UnmarshalJSON(b []byte) error {
	p := codec_flags_json{}
	if e := blame.O0(json.Unmarshal(b, &p)); e != nil {
		return e
	}

	c.DecodingSupported = p.DecodingSupported
	c.EncodingSupported = p.EncodingSupported
	c.Type = codec_type_from_human(p.Type)
	c.IntraFrameOnly = p.IntraFrameOnly
	c.Lossy = p.Lossy
	c.Losslss = p.Losslss

	return nil
}

func codec_type_from_letter(input string) *enum_codec_type {
	switch input {
	case "V":
		return VideoCodec
	case "A":
		return AudioCodec
	case "S":
		return SubtitleCodec
	case "D":
		return DataCodec
	case "T":
		return AttachmentCodec
	}
	return nil
}

func codec_type_from_human(input string) *enum_codec_type {
	switch input {
	case "Video Codec":
		return VideoCodec
	case "Audio Codec":
		return AudioCodec
	case "Subtitle Codec":
		return SubtitleCodec
	case "Data Codec":
		return DataCodec
	case "Attachment Codec":
		return AttachmentCodec
	}
	return nil
}

func codec_flags_from(input string) CodecFlags {
	output := CodecFlags{}
	parts := strings.Split(input, "")
	if len(parts) < 6 {
		return output
	}

	output.DecodingSupported = parts[0] == "D"
	output.EncodingSupported = parts[1] == "E"
	output.Type = codec_type_from_letter(parts[2])
	output.IntraFrameOnly = parts[3] == "I"
	output.Lossy = parts[4] == "L"
	output.Losslss = parts[5] == "S"

	return output
}

type Codec struct {
	Flags       CodecFlags
	Name        string
	Description string
}

var Codecs = func() []Codec {
	cmd := exec.Command(ffmpeg_filepath, "-codecs")
	var o bytes.Buffer
	cmd.Stdout = &o
	var e bytes.Buffer
	cmd.Stderr = &e

	run_res := blame.O0(cmd.Run())
	if run_res != nil {
		panic(run_res.WithAdditionalContext(e.String()))
	}
	o_s := o.String()
	header_and_body := strings.Split(o_s, "-------\n")
	body := header_and_body[1]
	lines := strings.Split(strings.TrimSpace(body), "\n")
	output := []Codec{}
	for _, line := range lines {
		l := nanolex.New(line)
		if pp, _ := l.Peek(-1); pp == ' ' {
			l.Read()
		}
		flags := l.ReadUntil(' ')
		l.ReadUntilNot(' ')

		name := l.ReadUntil(' ')
		l.ReadUntilNot(' ')

		description := l.ReadUntilEOF()
		// fmt.Println(flags, len(flags))

		codec := Codec{}
		codec.Flags = codec_flags_from(flags)
		codec.Name = name
		codec.Description = description
		output = append(output, codec)
	}

	return output
}()

var CodecLUT = func(input []Codec) map[string]Codec {
	output := map[string]Codec{}
	for _, codec := range input {
		output[codec.Name] = codec
	}

	return output
}(Codecs)

// TODO get this list dynamically
var Presets = []string{"ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow"}

type ETAAverage struct {
	Progress  time.Duration
	Cost      time.Duration
	Remaining time.Duration
	Speed     float64
	Samples   int
	N         int
}

type ETAProgress struct {
	Current time.Duration
	Total   time.Duration
}

type ETA struct {
	Average  ETAAverage
	Progress ETAProgress
}

type Int64Tracker interface {
	ID() string

	ETA() ETA

	SetTotal(v int64)
	GetTotal() int64
	SetValue(v int64)
	GetValue() int64
	SetTitle(v string)
	GetTitle() string

	Start(*os.Process)
	Cancel() error
	Finish(error) error
}

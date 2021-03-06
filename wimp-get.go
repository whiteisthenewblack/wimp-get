package main

import (
	"os"
	"fmt"
	"regexp"
	"encoding/json"
	"path"
	"io"
	"io/ioutil"
	"wimp-get/wimp"
	"wimp-get/platform"
	"os/exec"
	"net/http"
	"strings"
)

func SanitiseFilename(filename string) (newName string, e error) {
	r, e := regexp.Compile("[\\?<>:/\"\\\\|\\*]")
	if e != nil {
		return
	}

	newName = r.ReplaceAllString(filename, "")
	return
}

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: %s <wimp id or url>\n", os.Args[0])
		os.Exit(-1)
	}

	var id string;
	wimpUrl, _ := regexp.Compile("^https?://play\\.wimpmusic\\.com/album/")
	if wimpUrl.MatchString(os.Args[1]) {
		id = wimpUrl.ReplaceAllString(os.Args[1], "")
	} else {
		id = os.Args[1]
	}

	exe, _ := os.Executable()
	wDir, e := platform.DirOf(exe)
	if e != nil {
		panic(e)
	}

	magicData, e := ioutil.ReadFile(wDir + "/magic.json")
	if e != nil {
		panic(e)
	}

	var magic map[string]interface{}
	e = json.Unmarshal(magicData, &magic)
	if e != nil {
		panic(e)
	}

	println("Looking up album...")

	album, e := wimp.GetAlbum(id, magic["sessionId"].(string))
	if e != nil {
		panic(e)
	}

	fmt.Printf("[ %s - %s (%d) ]\n", album.Artist, album.Title, album.Year)

	// Determine whether we have more than one disc
	maxdisc := 0
	for _, track := range album.Tracks {
		if track.Volume > maxdisc {
			maxdisc = track.Volume
		}
	}
	var multidisc bool
	if maxdisc > 1 {
		multidisc = true
	}

	dirName := album.Artist+" - "+album.Title+" ("+fmt.Sprintf("%d", album.Year)+") [WEB FLAC]"
	dirName, e = SanitiseFilename(dirName)
	if e != nil {
		panic(e)
	}

	e = os.Mkdir(dirName, os.FileMode(0755))
	if e != nil {
		panic(e)
	}

	var firstFile string // we use this later to generate spectrals

	// Time to do the ripping!
	for i, track := range album.Tracks {
		num := fmt.Sprintf("%d", track.Number)
		if len(num) < 2 {
			num = "0"+num
		}

		fmt.Printf("[%d/%s] %s...", track.Volume, num, track.Title)

		var filename = fmt.Sprintf("%s - %s.flac", num, track.Title)
		filename, e = SanitiseFilename(filename)
		if e != nil {
			panic(e)
		}

		if (multidisc) {
			filename = fmt.Sprintf("%s/Disc %d/%s", dirName, track.Volume, filename)
		} else {
			filename = fmt.Sprintf("%s/%s", dirName, filename)
		}

		if i == 0 {
			firstFile = filename
		}

		// create disc dir if necessary
		if _, e = os.Stat(path.Dir(filename)); e != nil {
			if os.IsNotExist(e) {
				e = os.Mkdir(path.Dir(filename), os.FileMode(0755))
				if e != nil {
					panic(e)
				}
			} else {
				panic(e)
			}
		}

		resp, e := http.Get(track.Url)
		if e != nil {
			panic(e)
		}

		ffmpeg := exec.Command(magic["ffmpeg"].(string), "-i", "-", "-metadata", "title="+track.Title, "-metadata", "artist="+track.Artist,
			"-metadata", "album="+album.Title, "-metadata", "year="+fmt.Sprintf("%d", album.Year), "-metadata", "track="+fmt.Sprintf("%d", track.Number),
			"-metadata", "albumartist="+album.Artist, "-metadata", "discnumber="+fmt.Sprintf("%d", track.Volume), filename)

		stdin, e := ffmpeg.StdinPipe()
		if e != nil {
			panic(e)
		}

		e = ffmpeg.Start()
		if e != nil {
			panic(e)
		}

		_, e = io.Copy(stdin, resp.Body)
		if e != nil {
			panic(e)
		}

		resp.Body.Close()

		println(" Done!")
	}

	// Get art
	print("Getting album art...")

	resp, e := http.Get(album.CoverUrl)
	if e != nil {
		panic(e)
	}
	defer resp.Body.Close()

	var out io.Writer
	if multidisc {
		var outs []io.Writer
		for i := 1; i <= maxdisc; i++ {
			o, e := os.Create(fmt.Sprintf("%s/Disc %d/cover.jpg", dirName, i))
			if e != nil {
				panic(e)
			}
			outs = append(outs, o)
		}
		out = io.MultiWriter(outs...)
		e = nil
	} else {
		out, e = os.Create(dirName+"/cover.jpg")
	}
	if e != nil {
		panic(e)
	}

	_, e = io.Copy(out, resp.Body)
	if e != nil {
		panic(e)
	}

	println(" Done!")

	// Generate Spectrals
	var choice string
	for !(choice == "y" || choice == "n") {
		print("Generate spectrals? [y/n] ")
		fmt.Scanln(&choice)
		choice = strings.TrimRight(choice, "\n")
	}

	if choice == "y" {
		full := exec.Command(magic["sox"].(string), firstFile, "-n", "remix", "1", "spectrogram",
			"-x", "3000", "-y", "513", "-z", "120", "-w", "Kaiser", "-o", "SpecFull.png")
		zoom := exec.Command(magic["sox"].(string), firstFile, "-n", "remix", "1", "spectrogram",
			"-X", "500", "-y", "1025", "-z", "120", "-w", "Kaiser", "-S", "0:30", "-d", "0:04", "-o", "SpecZoom.png")
		e = full.Run()
		if e != nil {
			println("Error generating full spectral!")
		} else {
			println("SpecFull.png written")
		}
		e = zoom.Run()
		if e != nil {
			println("Error generating zoomed spectral!")
		} else {
			println("SpecZoom.png written")
		}
	}

	// Make .torrent file
	choice = ""
	for !(choice == "y" || choice == "n") {
		print("Create .torrent file? [y/n] ")
		fmt.Scanln(&choice)
		choice = strings.TrimRight(choice, "\n")
	}

	if choice == "y" {
		print("Announce URL: ")
		var announce string
		fmt.Scanln(&announce)
		announce = strings.TrimRight(announce, "\n")

		torrent := exec.Command(magic["mktorrent"].(string), "-l", "20", "-a", announce, dirName)
		e = torrent.Run()
		if e != nil {
			println("Error creating .torrent file!")
		} else {
			fmt.Printf("%s.torrent created\n", dirName)
		}
	}

	println("All done, exiting")
}

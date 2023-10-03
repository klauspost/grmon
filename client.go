package main

import (
	"bytes"
	"fmt"
	"net/http"
	"time"
)

var (
	client = &http.Client{Timeout: 10 * time.Second}
)

func getBody(url string) (bytes.Buffer, error) {
	var buf bytes.Buffer

	r, err := client.Get(url)
	if err != nil {
		return buf, err
	}
	defer r.Body.Close()

	_, err = buf.ReadFrom(r.Body)
	if err != nil {
		return buf, err
	}

	return buf, nil
}

var cachedRoutines *Routines

func poll() (routines Routines, err error) {
	if cachedRoutines != nil {
		// Deep Clone
		dst := make(Routines, 0, len(*cachedRoutines))
		for _, r := range *cachedRoutines {
			if len(r.Trace) == 0 {
				continue
			}
			dst = append(dst, &Routine{
				Num:       r.Num,
				State:     r.State,
				CreatedBy: r.CreatedBy,
				Trace:     append(make([]string, 0, len(r.Trace)), r.Trace...),
			})
		}
		return dst, nil
	}
	url := fmt.Sprintf("http://%s/%s/goroutine?debug=2", *hostFlag, *endpointFlag)
	buf, err := getBody(url)
	if err != nil {
		return
	}

	return ReadRoutines(&buf), nil
}

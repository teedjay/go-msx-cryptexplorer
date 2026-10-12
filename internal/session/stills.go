package session

import "image"

// Stills is the scripted run behind cmd/screenshot and the regression
// test: logo, title, demo, pyramid intro, play and pause, by frame.
var Stills = struct {
	Press string
	Shots []struct {
		Name  string
		Frame int
	}
}{
	Press: "1500:SPACE,1504:-SPACE,1560:SPACE,1564:-SPACE,1950:RIGHT,1990:-RIGHT,2010:F1,2014:-F1",
	Shots: []struct {
		Name  string
		Frame int
	}{
		{"1-logo", 150}, {"2-title", 450}, {"3-demo", 1400}, {"4-intro", 1800},
		{"5-play", 2000}, {"6-pause", 2040},
	},
}

// RunStills plays the scripted run and calls shot for each still.
func RunStills(shot func(name string, img *image.RGBA)) error {
	sc, err := ParsePress(Stills.Press)
	if err != nil {
		return err
	}
	s := New()
	held := Released()
	last := Stills.Shots[len(Stills.Shots)-1].Frame
	next := 0
	for s.Frame <= last {
		sc.Apply(s.Frame, &held)
		s.Step(held)
		if next < len(Stills.Shots) && s.Frame-1 == Stills.Shots[next].Frame {
			shot(Stills.Shots[next].Name, s.Draw())
			next++
		}
	}
	return nil
}

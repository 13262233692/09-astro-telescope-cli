package fits

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"astro-telescope-cli/pkg/model"
)

const (
	cardSize     = 80
	blockSize    = 2880
	cardsPerBlock = blockSize / cardSize
)

type HeaderCard struct {
	Keyword   string
	Value     string
	Comment   string
}

type Header struct {
	Cards []HeaderCard
}

func ParseHeaderFile(path string) (*Header, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseHeader(f)
}

func ParseHeader(r io.Reader) (*Header, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	hdr := &Header{Cards: make([]HeaderCard, 0, 64)}
	buf := make([]byte, cardSize)
	cardCount := 0

	for {
		n, err := io.ReadFull(br, buf)
		if err != nil && err != io.ErrUnexpectedEOF {
			if err == io.EOF && len(hdr.Cards) > 0 {
				break
			}
			return nil, fmt.Errorf("reading card: %w", err)
		}
		if n == 0 {
			break
		}

		card, err := parseCard(string(buf[:n]))
		if err != nil {
			return nil, err
		}
		hdr.Cards = append(hdr.Cards, card)
		cardCount++

		if card.Keyword == "END" {
			break
		}
	}

	return hdr, nil
}

func parseCard(s string) (HeaderCard, error) {
	if len(s) < cardSize {
		s = s + strings.Repeat(" ", cardSize-len(s))
	}

	card := HeaderCard{
		Keyword: strings.TrimSpace(s[0:8]),
	}

	if card.Keyword == "END" || card.Keyword == "" {
		return card, nil
	}

	if len(s) >= 10 && s[8] == '=' {
		valueStr := strings.TrimSpace(s[10:])
		if idx := strings.Index(valueStr, " / "); idx >= 0 {
			card.Comment = strings.TrimSpace(valueStr[idx+3:])
			valueStr = strings.TrimSpace(valueStr[:idx])
		} else if idx := strings.Index(valueStr, "/"); idx >= 0 && len(valueStr) > idx+1 {
			prev := valueStr[idx-1]
			if prev == ' ' {
				card.Comment = strings.TrimSpace(valueStr[idx+1:])
				valueStr = strings.TrimSpace(valueStr[:idx])
			}
		}
		card.Value = strings.Trim(valueStr, "'\"")
		card.Value = strings.TrimSpace(card.Value)
	}

	return card, nil
}

func (h *Header) Get(keyword string) (string, bool) {
	for _, c := range h.Cards {
		if strings.EqualFold(c.Keyword, keyword) {
			return c.Value, true
		}
	}
	return "", false
}

func (h *Header) GetFloat(keyword string) (float64, error) {
	v, ok := h.Get(keyword)
	if !ok {
		return 0, fmt.Errorf("keyword %s not found", keyword)
	}
	return strconv.ParseFloat(v, 64)
}

func (h *Header) GetInt(keyword string) (int, error) {
	v, ok := h.Get(keyword)
	if !ok {
		return 0, fmt.Errorf("keyword %s not found", keyword)
	}
	return strconv.Atoi(v)
}

func (h *Header) ToMap() map[string]string {
	m := make(map[string]string, len(h.Cards))
	for _, c := range h.Cards {
		if c.Keyword != "" && c.Keyword != "END" {
			m[c.Keyword] = c.Value
		}
	}
	return m
}

func (h *Header) ToCalibrationParams() (*model.CalibrationParams, error) {
	params := &model.CalibrationParams{
		Custom: make(map[string]string),
	}

	if v, ok := h.Get("DATE-OBS"); ok {
		if t, err := parseObsDate(v); err == nil {
			params.ObsDate = t
		}
	}

	if v, ok := h.Get("TELESCOP"); ok {
		params.Telescope = v
	}

	if v, ok := h.Get("INSTRUME"); ok {
		params.Instrument = v
	}

	if v, err := h.GetFloat("EXPTIME"); err == nil {
		params.Exposure = v
	} else if v, err := h.GetFloat("EXPOSURE"); err == nil {
		params.Exposure = v
	}

	if v, err := h.GetFloat("GAIN"); err == nil {
		params.Gain = v
	}

	if v, err := h.GetFloat("TEMP"); err == nil {
		params.Temp = v
	} else if v, err := h.GetFloat("DETTEMP"); err == nil {
		params.Temp = v
	}

	for _, c := range h.Cards {
		if c.Keyword != "" && c.Keyword != "END" {
			switch c.Keyword {
			case "DATE-OBS", "TELESCOP", "INSTRUME", "EXPTIME", "EXPOSURE", "GAIN", "TEMP", "DETTEMP":
				continue
			default:
				params.Custom[c.Keyword] = c.Value
			}
		}
	}

	return params, nil
}

func parseObsDate(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02T15:04:05.999",
		"2006-01-02T15:04:05",
		"2006-01-02",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.999Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("unrecognized date format: " + s)
}

func GenerateSampleFITSHeader(path string) error {
	lines := []string{
		"SIMPLE  =                    T / file does conform to FITS standard             ",
		"BITPIX  =                   16 / number of bits per data pixel                  ",
		"NAXIS   =                    2 / number of data axes                            ",
		"NAXIS1  =                  512 / length of data axis 1                          ",
		"NAXIS2  =                  512 / length of data axis 2                          ",
		"DATE-OBS= '2025-06-15T03:24:18' / date of observation                           ",
		"TELESCOP= 'VLA     '           / telescope name                                 ",
		"INSTRUME= 'EVLA    '           / instrument name                                ",
		"EXPTIME =               1800.0 / exposure time in seconds                       ",
		"GAIN    =                  2.5 / detector gain in e-/ADU                        ",
		"DETTEMP =                -150.0 / detector temperature in Celsius               ",
		"BAND    = 'L-Band  '           / frequency band                                 ",
		"FREQ    =           1420405752 / observation frequency in Hz                    ",
		"OBJECT  = 'M31     '           / observed object                                ",
		"RA      = '00:42:44.3'         / right ascension                                ",
		"DEC     = '+41:16:09'          / declination                                    ",
		"EQUINOX =               2000.0 / equinox of coordinates                         ",
		"OBSERVER= 'J.Smith '           / observer name                                  ",
		"END                                                                             ",
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, line := range lines {
		if len(line) < cardSize {
			line = line + strings.Repeat(" ", cardSize-len(line))
		} else if len(line) > cardSize {
			line = line[:cardSize]
		}
		_, err = f.WriteString(line)
		if err != nil {
			return err
		}
	}
	return nil
}

package tableview

import (
	"bytes"
	"encoding/xml"
	"image"
	// The formats image.DecodeConfig reads.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strconv"
	"strings"
)

// probeImage reads a PNG, JPEG, or GIF image's format and size.
func probeImage(reader headerReader, properties *Properties) {
	config, format, err := image.DecodeConfig(io.NewSectionReader(reader.file, 0, reader.size))
	if err != nil {
		return
	}
	properties.Format = strings.ToUpper(format)
	properties.Width, properties.Height = config.Width, config.Height
}

// probeSVG reads the width and height of an SVG's root element, or its
// viewBox when they are missing or relative.
func probeSVG(reader headerReader, properties *Properties) {
	decoder := xml.NewDecoder(bytes.NewReader(reader.read(0, 64<<10)))
	decoder.Strict = false
	for {
		token, err := decoder.Token()
		if err != nil {
			return
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "svg" {
			return
		}
		attributes := map[string]string{}
		for _, attribute := range start.Attr {
			attributes[attribute.Name.Local] = attribute.Value
		}
		width, widthOK := svgLength(attributes["width"])
		height, heightOK := svgLength(attributes["height"])
		if box := strings.Fields(strings.ReplaceAll(attributes["viewBox"], ",", " ")); (!widthOK || !heightOK) && len(box) == 4 {
			width, widthOK = svgLength(box[2])
			height, heightOK = svgLength(box[3])
		}
		if widthOK && heightOK {
			properties.Width, properties.Height = width, height
		}
		return
	}
}

// svgLength reads a length in user units or pixels; others are relative or
// need a conversion the viewer does not make.
func svgLength(text string) (int, bool) {
	text = strings.TrimSuffix(strings.TrimSpace(text), "px")
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return int(value + 0.5), true
}

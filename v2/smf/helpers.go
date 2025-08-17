package smf

import "io"

// dec2binDenom converts the decimal denominator to the binary one
// it works, use it!
func dec2binDenom(dec uint8) (bin uint8) {
	if dec <= 1 {
		return 0
	}
	for dec > 2 {
		bin++
		dec = dec >> 1

	}
	return bin + 1
}

// bin2decDenom converts the binary denominator to the decimal
func bin2decDenom(bin uint8) uint8 {
	if bin == 0 {
		return 1
	}
	return 2 << (bin - 1)
}

type readerCounter struct {
	r     io.Reader
	count int
}

func (r *readerCounter) Close() error {
	if cl, is := r.r.(io.ReadCloser); is {
		return cl.Close()
	}
	return nil
}

func (r *readerCounter) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	if n > 0 {
		r.count += n
	}
	return
}

package domain

// TOTPSetup is shown once, while the second factor is being enabled: the
// secret for typing it by hand, the otpauth:// URI and that URI as a QR code (SVG).
type TOTPSetup struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
	QR     string `json:"qr"`
}

package faceguard

func (r Retouch) ForPoints(n int) Retouch {
	if !r.Local() || n >= pupilPoints {
		return r
	}
	if n >= minPoints {
		r.EyeColor = ""
		return r
	}
	return Retouch{Light: r.Light, SkinTone: r.SkinTone, SkinLightness: r.SkinLightness}
}

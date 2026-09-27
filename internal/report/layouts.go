package report

// Set<Command> picks the text layout of a v0.2 command whose layout needs
// nothing beyond Data and Context.
func (r *Report) SetFreeze() { r.layout = (*Report).writeFreeze }

func (r *Report) SetTopObjects() { r.layout = (*Report).writeTopObjects }

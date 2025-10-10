package filters

type Filter struct {
	Accounts map[string]struct{}
	Programs map[string]struct{}
	Kind     string // "" means any
}

func New(accounts, programs []string, kind string) Filter {
	mk := func(xs []string) map[string]struct{} {
		m := make(map[string]struct{}, len(xs))
		for _, x := range xs { m[x] = struct{}{} }
		return m
	}
	return Filter{Accounts: mk(accounts), Programs: mk(programs), Kind: kind}
}

type EventMeta struct {
	Account string
	Program string
	Kind    string
}

func (f Filter) Match(m EventMeta) bool {
	if f.Kind != "" && f.Kind != m.Kind { return false }
	if len(f.Accounts) > 0 {
		if _, ok := f.Accounts[m.Account]; !ok { return false }
	}
	if len(f.Programs) > 0 {
		if _, ok := f.Programs[m.Program]; !ok { return false }
	}
	return true
}

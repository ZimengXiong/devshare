package main

var adjectives = []string{"amber", "brisk", "calm", "clear", "gentle", "lilac", "quiet", "rapid", "silver", "small", "soft", "solar", "vivid", "warm"}

var nouns = []string{"brook", "cedar", "comet", "dawn", "field", "harbor", "lake", "meadow", "orbit", "otter", "panda", "pine", "river", "sparrow"}

func (s *Server) newNames() (string, string) {
	for {
		suffix := randomText(2)
		h := adjectives[int(suffix[0])%len(adjectives)] + "-" + nouns[int(suffix[1])%len(nouns)] + "-" + suffix
		var n int
		if s.db.QueryRow(`SELECT count(*) FROM shares WHERE hostname=?`, h+"."+s.cfg.SiteDomain).Scan(&n) == nil && n == 0 {
			return "shr_" + randomText(12), h + "." + s.cfg.SiteDomain
		}
	}
}

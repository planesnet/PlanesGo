package llm

import "strings"

// legacyPlanPrune: la poda por niveles de antes de la vigencia, tal cual (con ella se calibra la vigencia en
// TestVigenciaCalibration). young: intercambios de L1 (los últimos).
func legacyPlanPrune(win []*ocMessage, sims *pruneSims, young int) prunePlan {
	xs := pruneExchanges(win)
	plan := prunePlan{Edits: map[string]pruneEdit{}, Exchanges: xs}
	// Lo que pasa después de cada parte: lecturas, ediciones y órdenes posteriores (en cualquier nivel).
	type pos struct{ x, m, p int }
	var orden []pos
	for i, x := range xs {
		for j, m := range x.Messages {
			for k := range m.Parts {
				orden = append(orden, pos{i, j, k})
			}
		}
	}
	part := func(q pos) *ocPart { return xs[q.x].Messages[q.m].Parts[q.p] }
	lastRead, lastWrite, lastCmd, lastTodo := map[string]int{}, map[string]int{}, map[string]int{}, -1
	for n, q := range orden {
		p := part(q)
		if p.v.Type != "tool" {
			continue
		}
		switch p.v.Tool {
		case "read":
			lastRead[p.filePath()] = n
		case "edit", "write", "patch", "apply_patch", "multiedit":
			lastWrite[p.filePath()] = n
		case "bash":
			lastCmd[strings.TrimSpace(resumen(p.v.State.Input))] = n
		case "todowrite":
			lastTodo = n
		}
	}
	first := 0
	for i, x := range xs {
		switch {
		case i >= len(xs)-young:
			x.Tier = TierL1
		case i == first || sims == nil:
			x.Tier = TierL2
		default:
			s, ok := sims.Exchange[x.UserID]
			if !ok {
				// Sin su similitud (Ollama no responde): lo que estaba fuera sigue fuera (reponerlo rompería la caché).
				if sims.PrevTier[x.UserID] == TierL3 {
					x.Tier = TierL3
				}
				break
			}
			x.Sim = s
			limite := pruneThreshold
			if sims.PrevTier[x.UserID] == TierL3 {
				limite += pruneReviveMargin
			}
			if s < limite {
				x.Tier = TierL3
			}
		}
	}
	for n, q := range orden {
		x := xs[q.x]
		if x.Tier == TierL1 {
			continue
		}
		m := x.Messages[q.m]
		p := part(q)
		e, ok := legacyPruneRule(p, m, x.Tier, n, lastRead, lastWrite, lastCmd, lastTodo)
		if !ok && x.Tier == TierL2 && sims != nil && p.isPiece() {
			if s, hay := sims.Piece[p.ID]; hay && s < pruneThreshold {
				e, ok = pruneEdit{Clear: true}, true
			}
		}
		if !ok {
			continue
		}
		plan.Edits[p.ID] = e
	}
	return plan
}

// pruneRule: las reglas seguras de una parte de L2, o en L3 su línea indicativa.
func legacyPruneRule(p *ocPart, m *ocMessage, tier pruneTier, n int, lastRead, lastWrite, lastCmd map[string]int, lastTodo int) (pruneEdit, bool) {
	if p.v.Type == "reasoning" {
		// El razonamiento de turnos anteriores, salvo si es lo único del mensaje (quedaría vacío y Gemini lo rechaza).
		for _, o := range m.Parts {
			if o.v.Type == "tool" || (o.v.Type == "text" && strings.TrimSpace(o.v.Text) != "") {
				return pruneEdit{Delete: true}, true
			}
		}
		return pruneEdit{}, false
	}
	if !p.isTool() {
		return pruneEdit{}, false
	}
	trim := p.v.Tool == "edit" || p.v.Tool == "write" || p.v.Tool == "patch" || p.v.Tool == "apply_patch" || p.v.Tool == "multiedit"
	if tier == TierL3 {
		if trim {
			return pruneEdit{TrimInput: true}, true
		}
		if len(p.v.State.Output) < pruneSmall && len(p.v.State.Attachments) == 0 {
			return pruneEdit{}, false
		}
		return pruneEdit{Clear: true}, true
	}
	out := p.output()
	switch p.v.Tool {
	case "edit", "write", "patch", "apply_patch", "multiedit":
		return pruneEdit{TrimInput: true}, true
	case "read":
		f := p.filePath()
		if (lastRead[f] > n) || lastWrite[f] > n {
			return pruneEdit{Clear: true}, true
		}
		if len(p.v.State.Attachments) > 0 {
			return pruneEdit{Clear: true}, true
		}
		return pruneEdit{}, false
	case "bash":
		if lastCmd[strings.TrimSpace(resumen(p.v.State.Input))] > n && len(p.v.State.Output) >= pruneSmall {
			return pruneEdit{Clear: true}, true
		}
	case "todowrite":
		if lastTodo > n && len(p.v.State.Output) >= pruneSmall {
			return pruneEdit{Clear: true}, true
		}
		return pruneEdit{}, false
	}
	if len(p.v.State.Attachments) > 0 {
		return pruneEdit{Clear: true}, true
	}
	if c, ok := cutLong(out); ok {
		return pruneEdit{Output: &c}, true
	}
	return pruneEdit{}, false
}

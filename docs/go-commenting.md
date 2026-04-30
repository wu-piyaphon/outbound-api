# Go Commenting Standards

This guide defines when and how to write comments in the outbound-api codebase.

## Core Principle

Comment **why** the code does something, never **what** it does. The code already
shows what it does. Comments exist to capture intent, constraints, and trade-offs
that a future reader cannot easily infer from the code alone.

---

## When to Comment

### 1. All exported symbols — GoDoc style

Every exported type, interface, function, constant, and variable must have a GoDoc
comment. Start with the symbol name, use a complete sentence, end with a period.

```go
// TradeService orchestrates order placement and trade lifecycle management.
type TradeService interface { ... }

// ExecuteBuyTrade places a market buy order and persists the resulting trade
// within a single database transaction. Returns nil, nil when the account has
// no remaining trade slots.
func (t *tradeService) ExecuteBuyTrade(...) (*model.Trade, error) { ... }
```

### 2. Non-obvious business rules and domain formulas

Comment the *intent* behind trading logic, risk formulas, and state transitions.
Readers unfamiliar with the five-layer strategy or position-sizing math need this.

```go
// Stop distance = ATR × ATRRiskMultiplier.
// This matches the actual stop-loss placement below so risk is internally consistent.
atrStopDistance := signal.Indicators.ATR.Mul(t.atrRiskMultiplier)
```

### 3. Idempotency and defensive guard clauses

Comment any path that exists specifically to prevent double-execution, replay, or
data corruption—these guards are invisible to a reader scanning the happy path.

```go
if trade.Status == model.StatusFilled { ... }
    return nil // already terminal, ignore replay
}
```

### 4. Concurrency boundaries

Comment goroutine ownership, channel direction, and cancellation expectations.

```go
// barChan is owned by the stream client; workers are read-only consumers.
barChan := make(chan stream.Bar)
```

### 5. Operational or performance-sensitive decisions

Comment choices that were made deliberately to avoid cost or latency, so future
engineers don't "fix" them back.

```go
// Seed indicator cache once per symbol at startup — zero REST calls in hot path.
```

### 6. Non-trivial default/fallback branches

Comment what condition triggers a fallback and why the fallback is safe.

```go
} else {
    // Fallback: equal-weight sizing when ATR is unavailable.
```

---

## When Not to Comment

| Situation | Why |
|---|---|
| Trivial assignment (`x = y`) | Code already says this |
| Restating the function name in prose | Adds no information |
| Describing `if err != nil { return err }` | Universal Go pattern |
| Change history or authorship | Use git blame |
| TODOs without owner + actionable next step | Creates noise; prefer an issue |

---

## GoDoc Formatting Rules

- Start with the **symbol name exactly as declared**, then a verb. `// TradeService orchestrates...`
- For functions, describe the contract: inputs, outputs, side effects, error conditions.
- For interfaces, describe the role of the implementer.
- For types/structs, describe the lifecycle and ownership if non-obvious.
- For constants/enums, add an inline comment `// brief description (terminal)` when the value carries semantic meaning.

---

## Layer-Specific Guidance

| Package | Comment priority |
|---|---|
| `internal/service` | Trading strategy layers, risk formulas, state-transition guards |
| `internal/repository` | Non-obvious query semantics (e.g. conditional UPDATE guards) |
| `internal/alpaca` | Event mapping assumptions, feed selection, channel ownership |
| `internal/indicator` | Lifecycle (seed-once, replace-on-re-seed), thread safety |
| `internal/bot` | Atomic state transitions, valid state-change paths |
| `internal/model` | Enum lifecycle semantics (pending → open → terminal) |
| `cmd/server/main.go` | Goroutine ownership, startup ordering, shutdown sequence |
| `*_test.go` | Only comment unusual fixtures or domain-specific test invariants |

---

## Quick Checklist (for PRs)

- [ ] All new exported symbols have a GoDoc comment.
- [ ] No comment merely restates the code in prose.
- [ ] Non-obvious business rules, formulas, or fallbacks are explained.
- [ ] Concurrency boundaries (goroutines, channels) are documented.
- [ ] TODOs include context and are linked to a tracking issue.

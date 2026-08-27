# Upstream integration notes

The core package intentionally has no dependency on `golang.org/x/tools`. `PlanFile` returns byte offsets so the same engine can be wired into `funcorder` without coupling the arranger to a particular analyzer version.

A `funcorder` analyzer integration can follow this shape:

```go
src, err := pass.ReadFile(filename)
if err != nil {
    return err
}

plan, err := reorder.PlanFile(filename, src, cfg)
if err != nil {
    // ErrUnsafeTrivia means: keep the diagnostic, omit the SuggestedFix.
    return err
}
if !plan.Changed() {
    return nil
}

edit := plan.Edit
file := pass.Fset.File(astFile.Pos())
fix := analysis.SuggestedFix{
    Message: "reorder declarations",
    TextEdits: []analysis.TextEdit{{
        Pos:     file.Pos(edit.Start),
        End:     file.Pos(edit.End),
        NewText: edit.NewText,
    }},
}
```

The exact upstream patch should keep `funcorder` as the diagnostic authority and attach at most one file-level reorder suggestion for a set of ordering diagnostics. This avoids overlapping SuggestedFix edits.

## Historical regression to retain

`funcorder` v0.4.0 added `--fix` and was retracted after issue #32 demonstrated that comments inside moved constructors/methods could disappear. Any upstream contribution based on this project should port the regression tests in this repository before enabling autofix.

## Contribution strategy

1. Keep rule classification in `funcorder` authoritative.
2. Reuse or transplant only the reorder planning/source-block logic.
3. Add `RunWithSuggestedFixes` fixtures for every enabled rule combination.
4. Add the historical #32 cases before enabling the fix in golangci-lint.
5. Keep unsafe standalone-comment cases diagnostic-only rather than forcing a rewrite.

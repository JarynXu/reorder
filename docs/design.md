# Design

## Goal

Provide a safe, deterministic autofix for the declaration-order rules that `funcorder` already enforces. The arranger must not define a second style language and must not change program semantics merely to make a file look cleaner.

## Compatibility boundary

The implementation follows the behavior of `funcorder` v0.6.0/current rule model:

1. A constructor is an exported, non-method function whose name starts with `New` or `Must` (case-insensitive prefix check), has at least one result, and whose first result resolves to an identifier or pointer-to-identifier.
2. Constructor checks only apply when that returned type is declared in the same file.
3. Constructor ordering requires the type declaration before the constructor and constructors before that type's methods.
4. Exported methods precede unexported methods for a type declared in the same file.
5. When `alphabetical` is enabled, constructors and the exported/unexported method groups are each sorted by name, but only when their owning constructor/method rule is enabled.
6. When `function` is enabled, exported top-level functions precede unexported top-level functions. `init` is excluded.
7. No additional rule is inferred from TODOs or style guides.

This intentionally preserves current edge behavior, including current generic-receiver recognition limits, because an autofix must agree with its diagnostic source before it tries to improve it.

## Source model

The parser is used for classification and ordering constraints only. A movable declaration is represented by its exact source byte span:

- attached doc comments are included;
- the complete function body is included verbatim;
- a same-line trailing comment after the declaration is included.

This avoids the historical failure mode where printing a standalone `ast.FuncDecl` omitted comments that were stored in the file's comment list rather than inside that node.

## Anchors

The following declarations are anchors and never reorder relative to one another:

- imports;
- `const` declarations;
- `var` declarations;
- type declarations;
- `init` functions;
- other non-function top-level declarations.

Only ordinary function declarations and methods can move around anchors. This is intentionally stricter than a general AST sorter because package initialization order can be affected by declaration order, while moving ordinary function declarations is semantically inert.

## Constraint graph

Each top-level declaration is a graph vertex. Directed edges express required order:

- anchor -> next anchor;
- type -> constructor;
- constructor -> each method of that type;
- each exported method -> each unexported method of that type;
- alphabetical adjacent edges within enabled groups;
- each exported top-level function -> each unexported top-level function.

A stable topological sort chooses the lowest original source index whenever multiple vertices are available. This makes the result deterministic while preserving source order whenever the rules do not require movement.

## Comment barriers

Whitespace between declarations is safe to cross. Non-whitespace trivia that is not already part of a declaration's source span is ambiguous: it may be a section comment, directive, or intentionally detached note.

Before producing an edit, the planner checks every source boundary crossed by the new declaration order. If a crossed boundary contains non-whitespace trivia, planning fails with `ErrUnsafeTrivia`. The caller can then leave the linter diagnostic for manual resolution.

## Edit shape

The planner returns one contiguous edit covering the smallest changed declaration range. Original whitespace slots inside that range are preserved byte-for-byte; only declaration blocks are permuted.

This shape is useful both for the CLI and for `analysis.SuggestedFix` integration.

## Invariants

Every change should preserve these properties:

- valid Go syntax;
- deterministic output;
- idempotency (`reorder(reorder(src)) == reorder(src)`);
- function body bytes preserved exactly;
- doc comments and same-line trailing comments move with their declaration;
- anchor relative order preserved;
- `init` relative order preserved;
- no crossing of unattached non-whitespace trivia without explicit human handling;
- a compliant file remains byte-identical.

// Package spans implements edwood's spans-protocol — the wire format,
// in-memory storage, and rich.Content render bridge that external tools
// use to drive styled text rendering in window bodies.
//
// External tools (md2spans, edcolor, dirthumb, etc.) write spans-
// protocol messages to a window's `spans` 9P file. Edwood parses each
// message with [ParseMessage], applies the resulting per-run styling
// and structural regions to a window's [Store] and [RegionStore], and
// converts the combined state into a rich.Content via [Render] for the
// styled-mode renderer.
//
// This package contains the protocol-pure code that has no dependency
// on edwood's window orchestration. The 9P endpoint glue and the
// styled-mode lifecycle remain in the main package.
//
// # Wire format
//
// Each Twrite to the spans file is one of:
//
//   - `c` — clear all styling and regions.
//   - `s OFFSET LENGTH FG [BG] [flags...]` — styled rune run.
//   - `b OFFSET LENGTH WIDTH HEIGHT FG BG [flags...] [payload...]` — replaced
//     element (image, fixed-dimension box).
//   - `begin region <kind> [params...]` / `end region` — nestable
//     structural region (code, blockquote, listitem, table, tablerow,
//     tablecell).
//
// Style flags include `bold`, `italic`, `hidden`, `scale=N.N`,
// `family=code`, `placement=below`, and `hrule`.
//
// See docs/designs/spans-protocol.md for the full specification.
package spans

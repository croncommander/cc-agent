# Bolt's Journal

## 2024-05-22 - [Initialization] **Learning:** Initialized Bolt's journal. **Action:** Record critical performance learnings here.

## 2025-12-18 - [JSON Unmarshalling] **Learning:** Polymorphic JSON messages (flat structure) often lead to double parsing (once for type, once for content). **Action:** Use a "Unified" struct containing all possible fields to allow single-pass unmarshalling when fields do not collide.

## 2024-05-22 - [Buffer String Construction] **Learning:** Using `strings.Builder` with `Grow` is significantly faster than `bytes.Buffer` for constructing strings from parts, as `strings.Builder.String()` avoids the final allocation. **Action:** Prefer `strings.Builder` over `bytes.Buffer` when the final goal is a `string`.

## 2026-01-02 - [String Scanning] **Learning:** `strings.IndexByte` uses assembly/SIMD and is ~15-17x faster than byte-by-byte loops for long strings, but slightly slower (~10%) for very short strings with dense matches due to function call and slicing overhead. **Action:** Prefer `strings.IndexByte` for scanning unless the input is known to be tiny and heavily populated with the search target.

# Bolt's Journal

## 2024-05-22 - [Initialization] **Learning:** Initialized Bolt's journal. **Action:** Record critical performance learnings here.

## 2025-12-18 - [JSON Unmarshalling] **Learning:** Polymorphic JSON messages (flat structure) often lead to double parsing (once for type, once for content). **Action:** Use a "Unified" struct containing all possible fields to allow single-pass unmarshalling when fields do not collide.

## 2024-01-01 - [Zero-Allocation Buffer] **Learning:** `limitedBuffer.WriteString` was allocating `[]byte(s)` unnecessarily. Go's `copy(dst []byte, src string)` allows direct copying. Also, `bytes.Buffer` in `String()` can be replaced by `strings.Builder` with `Grow()` to avoid redundant allocations. **Action:** Use `copy` for string-to-byte writes and `strings.Builder` for string construction when size is known.

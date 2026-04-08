# Bolt's Journal

## 2024-05-22 - [Initialization] **Learning:** Initialized Bolt's journal. **Action:** Record critical performance learnings here.

## 2025-12-18 - [JSON Unmarshalling] **Learning:** Polymorphic JSON messages (flat structure) often lead to double parsing (once for type, once for content). **Action:** Use a "Unified" struct containing all possible fields to allow single-pass unmarshalling when fields do not collide.

## 2024-05-22 - [Buffer String Construction] **Learning:** Using `strings.Builder` with `Grow` is significantly faster than `bytes.Buffer` for constructing strings from parts, as `strings.Builder.String()` avoids the final allocation. **Action:** Prefer `strings.Builder` over `bytes.Buffer` when the final goal is a `string`.

## 2026-01-29 - [System Call Hoisting] **Learning:** `os.Executable()` on Linux performs a `readlink` syscall which is not cached by the Go runtime. In a loop, this syscall overhead dominates execution time (e.g., ~2.8µs vs ~0.35µs per op). **Action:** Hoist `os.Executable()` and similar system calls (like `os.Getwd`) out of hot loops.

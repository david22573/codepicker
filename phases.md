# Codepicker: Critical Issues & Refactoring Phases

**Generated:** 2025-01-10  
**Status:** Phase 1 & 2 Complete ✅ | Phase 3 Pending 📋

---

## 🔴 Phase 1: Critical Safety & Security Issues
**Status:** ✅ COMPLETED

---

## 🟡 Phase 2: Architecture & Design Issues
**Status:** ✅ COMPLETED

### Improvements Implemented
* **Dependency Injection:** Scanner and Writer now accept dependencies (Logger, Config) explicitly.
* **Path Logic Separation:** Path security and validation moved to `internal/paths`.
* **Logger Interface:** Replaced global log variables with a proper `Logger` interface.
* **Minifier Strategy:** Refactored monolithic minifier into a extensible Registry pattern.

---

## 🟢 Phase 3: Code Quality & Testing (Next Up)

**Priority:** MEDIUM - Polish and best practices  
**Status:** 📋 NOT STARTED  
**Time Estimate:** 3-5 days  
**Effort:** Medium

### Issues to Address

| # | Issue | Impact | Fix Difficulty |
|---|-------|--------|----------------|
| 1 | **Magic Numbers Everywhere** | Hard to maintain constants (buffer sizes, limits) | Easy |
| 2 | **Inefficient String Building** | Performance degradation on large files | Easy |
| 3 | **Hardcoded UI Strings** | Can't test output, no i18n support | Easy |
| 4 | **Heavy `os.Exit()` Usage** | Makes code impossible to unit test safely | Hard |
| 5 | **Missing Tests** | Core logic (Scanner, Minifier) has 0% coverage | Medium |
| 6 | **Lack of Error Types** | Generic errors make handling specific failures hard | Medium |

### Proposed Improvements

#### 1. Constants Package
Create `internal/constants/constants.go` to hold all magic numbers:
```go
const (
    MinAPIKeyLength     = 10
    MaxFileSizeBytes    = 100 * 1024 * 1024
    MaxAPIRetries       = 3
    DefaultModel        = "xiaomi/mimo-v2-flash:free"
)

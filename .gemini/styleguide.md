# Code Review Style Guide

## ⚠️ CRITICAL: Consolidated Review Output Only

**DO NOT post individual inline code comments.**

Instead, consolidate **ALL** code review findings—including file names, line numbers, severity, and code suggestions—into a **single, well-structured Markdown report** within the main Pull Request Review Summary.

This ensures automation can fetch exactly one comment via GraphQL that contains the entire review state.

### Report Format (use in PR Summary)

For each finding, include:

- **File:** path relative to repo root  
- **Line:** line number(s)  
- **Severity:** HIGH | MEDIUM | LOW  
- **Description:** brief explanation  
- **Suggestion:** code block with recommended change (if applicable)

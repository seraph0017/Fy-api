---
name: report-docx
description: Use when creating a Word .docx report from TraceNex/Fy-api benchmark outputs, especially fy-poc-loadtest JSON/CSV/Markdown reports, customer-facing performance reports, or screenshot-friendly test summaries that need a formal docx deliverable.
---

# Report DOCX

Create customer-facing `.docx` reports from benchmark artifacts.

## Workflow

1. Locate source artifacts:
   - Prefer `fy-poc-loadtest` JSON when available: `poc_loadtest_*.json`.
   - Use Markdown/CSV only when JSON is unavailable.
2. Generate DOCX with the bundled script:
   ```bash
   python .agents/skills/report-docx/scripts/json_to_docx.py \
     --input <poc_loadtest.json> \
     --output <report.docx>
   ```
3. Validate:
   - Confirm the `.docx` exists and is non-empty.
   - Open or inspect if requested.
4. In the final response, link the generated file path.

## Style

- Chinese report structure by default.
- Include:
  - 标题
  - 测评基本信息
  - 测试方法
  - 总体结论
  - 短文本并发结果
  - 长文本基线结果
  - 风险与建议
- Keep tables compact; use average and p95 values.
- Do not include API keys, DSNs, or raw private prompts unless explicitly requested.

## Notes

- The script uses `python-docx`; if missing, install it in the active venv.
- For non-`fy-poc-loadtest` JSON schemas, adapt the script or write a small transformer rather than hand-building DOCX every time.

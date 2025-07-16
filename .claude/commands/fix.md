# 🐛 Bug Fix Workflow with TDD

Fix the following issue: $ARGUMENTS

## Phase 1: Bug Analysis

[AGENT: ARCHITECT]
1. Analyze the bug report/error message
2. Identify affected components
3. Trace the execution flow
4. Determine root cause
5. Assess impact scope

**Output Required:**
- Root cause analysis
- Affected components list
- Risk assessment
- Estimated complexity

## Phase 2: Test-First Fix (RED)

[AGENT: TEST_ENGINEER]

### Write Failing Test First
1. Create a test that reproduces the bug
2. Ensure the test fails with the current code
3. Test should clearly demonstrate the issue
4. Include edge cases related to the bug

**Critical Rule**: DO NOT fix the bug yet!

Example structure:
```javascript
describe('Bug: $ARGUMENTS', () => {
  it('should [expected behavior]', () => {
    // Test that currently fails
  });
  
  it('should handle [edge case]', () => {
    // Related edge case
  });
});
```

## Phase 3: Minimal Fix (GREEN)

Now implement the minimal fix:

1. Fix ONLY what's necessary to pass the test
2. Don't refactor other code
3. Don't add new features
4. Preserve existing functionality

[AGENT: TEST_ENGINEER]
- Run ALL tests (not just the new one)
- Ensure no regression
- Verify the bug is fixed

## Phase 4: Security & Performance Check

[AGENT: SECURITY_AUDITOR]
Review the fix for:
- New security vulnerabilities introduced
- Input validation if applicable
- Authorization checks if applicable

[AGENT: PERFORMANCE_OPTIMIZER]
Check if the fix:
- Introduces performance degradation
- Creates new bottlenecks
- Needs optimization

## Phase 5: Comprehensive Testing

[AGENT: TEST_ENGINEER]

Expand test coverage:
1. Add more test cases around the fix
2. Test integration with other components
3. Add regression tests
4. Verify edge cases

Target: 100% coverage for the fixed code path

## Phase 6: Code Review

[AGENT: CODE_REVIEWER]

Review the fix for:
- ✓ Solves the root cause (not symptoms)
- ✓ No unnecessary changes
- ✓ Follows coding standards
- ✓ Clear and maintainable
- ✓ Properly documented

## Phase 7: Documentation

[AGENT: DOCUMENTATION_WRITER]

Update:
1. Add comments explaining the fix (if complex)
2. Update changelog
3. Document any behavior changes
4. Add to known issues/fixes log

## 📋 Fix Summary

### Bug Details
- **Issue**: [Description]
- **Root Cause**: [Analysis]
- **Fix Applied**: [Summary]
- **Tests Added**: [Count]

### Verification Checklist
- [ ] Bug reproduced with test
- [ ] Test fails before fix
- [ ] Test passes after fix
- [ ] All existing tests pass
- [ ] No performance regression
- [ ] No security issues introduced
- [ ] Code reviewed
- [ ] Documentation updated

### Commit Message
Generate commit message:
```
fix(scope): brief description

- Root cause: [explanation]
- Solution: [what was changed]
- Tests: [what tests were added]

Fixes #[issue-number]
```

## 🚨 Regression Prevention

[AGENT: TEST_ENGINEER]
Suggest additional measures:
1. Related areas to test
2. Monitoring to add
3. Preventive refactoring needed

---

Fix complete. Review changes before committing.
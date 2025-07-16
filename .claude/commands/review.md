# 🔍 Comprehensive Code Review

Initiate a multi-agent code review for: $ARGUMENTS

## Review Scope Configuration

Perform comprehensive analysis across all quality dimensions:
- ✅ Code Quality & Maintainability
- ✅ Security Vulnerabilities
- ✅ Performance Optimization
- ✅ Test Coverage & Quality
- ✅ Documentation Completeness

## 👁️ Code Quality Review

[AGENT: CODE_REVIEWER]

### Structural Analysis
- **Readability**: Analyze naming conventions, code organization, and clarity
- **Complexity**: Check cyclomatic complexity and cognitive load
- **Maintainability**: Verify SOLID principles adherence
- **Design Patterns**: Identify proper pattern usage and anti-patterns

### Code Smells Detection
Check for:
- Long methods/functions
- Large classes
- Duplicate code
- Dead code
- Feature envy
- God objects
- Inappropriate intimacy

### Best Practices Compliance
- DRY (Don't Repeat Yourself)
- KISS (Keep It Simple, Stupid)
- YAGNI (You Aren't Gonna Need It)
- Single Responsibility Principle
- Error handling patterns

## 🔒 Security Audit

[AGENT: SECURITY_AUDITOR]

### Vulnerability Assessment
- **Input Validation**: Check all user inputs
- **SQL Injection**: Analyze database queries
- **XSS Prevention**: Review output encoding
- **CSRF Protection**: Verify token implementation
- **Authentication**: Review auth mechanisms
- **Authorization**: Check access controls
- **Sensitive Data**: Identify exposure risks

### Security Checklist
- [ ] All inputs validated and sanitized
- [ ] Parameterized queries used
- [ ] Proper output encoding
- [ ] Secure session management
- [ ] Secrets properly managed
- [ ] Rate limiting implemented
- [ ] Audit logging present

## ⚡ Performance Analysis

[AGENT: PERFORMANCE_OPTIMIZER]

### Performance Metrics
- **Algorithm Complexity**: O(n) analysis
- **Database Queries**: N+1 problems, missing indexes
- **Memory Usage**: Leaks, excessive allocations
- **Caching**: Opportunities and implementation
- **API Calls**: Unnecessary requests
- **Resource Loading**: Bundle sizes, lazy loading

### Optimization Recommendations
Provide specific, actionable improvements:
- Query optimization suggestions
- Caching strategies
- Algorithm improvements
- Resource bundling options

## 🧪 Test Quality Review

[AGENT: TEST_ENGINEER]

### Test Coverage Analysis
- **Line Coverage**: Current percentage
- **Branch Coverage**: Decision point coverage
- **Function Coverage**: Untested functions
- **Integration Coverage**: API endpoint coverage

### Test Quality Assessment
- Test naming clarity
- Test independence
- Mock usage appropriateness
- Edge case coverage
- Error scenario testing
- Performance test presence

## 📝 Documentation Review

[AGENT: DOCUMENTATION_WRITER]

### Documentation Completeness
- [ ] README up to date
- [ ] API documentation complete
- [ ] Code comments appropriate
- [ ] Examples provided
- [ ] Setup instructions clear
- [ ] Troubleshooting guide present

### Documentation Quality
- Clarity and accuracy
- Technical depth appropriate
- Examples functional
- Diagrams where helpful

## 🎯 Review Summary

### Critical Issues (Must Fix)
🔴 List all critical findings that block deployment

### High Priority (Should Fix)
🟠 Important issues that should be addressed soon

### Medium Priority (Consider Fixing)
🟡 Issues that impact maintainability

### Low Priority (Nice to Have)
🔵 Minor improvements and suggestions

### Positive Findings
✅ Highlight what's done well

## 📊 Overall Score

Provide ratings:
- **Code Quality**: [A-F]
- **Security**: [A-F]
- **Performance**: [A-F]
- **Test Coverage**: [percentage]%
- **Documentation**: [A-F]

**Overall Grade**: [A-F]

## 🔧 Actionable Next Steps

1. Immediate actions required
2. Short-term improvements
3. Long-term refactoring suggestions

## 💡 Quick Fixes

Provide specific code snippets for immediate improvements:
```
// Example fixes with explanations
```

---

Review completed. Use `/fix` to address issues or `/approve` to proceed.
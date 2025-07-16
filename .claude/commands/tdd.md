# Test-Driven Development Workflow with Sub-Agents

You are now initiating a comprehensive TDD workflow for: $ARGUMENTS

## 🤖 Sub-Agent Protocol

You will coordinate the following specialized sub-agents throughout this workflow:

- **[AGENT: ARCHITECT]** - System design and architecture analysis
- **[AGENT: TEST_ENGINEER]** - Test strategy and implementation
- **[AGENT: SECURITY_AUDITOR]** - Security vulnerability assessment
- **[AGENT: PERFORMANCE_OPTIMIZER]** - Performance analysis and optimization
- **[AGENT: DOCUMENTATION_WRITER]** - Documentation creation
- **[AGENT: CODE_REVIEWER]** - Code quality review

## 📋 Workflow Execution

### Phase 1: EXPLORATION (No Implementation)

[AGENT: ARCHITECT]
1. Analyze the codebase for implementing: $ARGUMENTS
2. Read all relevant files to understand current architecture
3. Identify dependencies and integration points
4. Map out affected components
5. List potential challenges and risks

**Output Required:**
- Architecture overview
- Integration points
- Risk assessment
- Complexity estimate

### Phase 2: PLANNING

[AGENT: ARCHITECT]
Create a detailed step-by-step implementation plan:
- Break down into testable increments
- Define clear acceptance criteria
- Identify critical path

[AGENT: TEST_ENGINEER]
Design comprehensive test strategy:
- List all test scenarios (unit, integration, e2e)
- Identify edge cases and error conditions
- Define test data requirements
- Plan test execution order

**Checkpoint:** Get user approval before proceeding

### Phase 3: TEST IMPLEMENTATION (RED)

[AGENT: TEST_ENGINEER]
Write ALL tests before any implementation:

1. Start with unit tests for core logic
2. Add integration tests for API/service boundaries
3. Include edge cases and error scenarios
4. Ensure tests are descriptive and maintainable
5. Verify all tests fail with appropriate messages

**Rules:**
- DO NOT write any implementation code
- Tests must be comprehensive
- Each test must have a clear purpose
- Use descriptive test names

### Phase 4: IMPLEMENTATION (GREEN)

Now implement the feature to make ALL tests pass:

1. Write the MINIMAL code needed
2. Focus only on passing tests
3. Don't add extra features
4. Handle all test cases
5. No premature optimization

[AGENT: TEST_ENGINEER]
After each implementation step:
- Run tests
- Report status
- Identify failing tests

**Success Criteria:** All tests must pass

### Phase 5: REFACTORING

[AGENT: PERFORMANCE_OPTIMIZER]
Analyze implementation for:
- Algorithmic complexity
- Memory usage
- Database query optimization
- Caching opportunities

[AGENT: CODE_REVIEWER]
Refactor for:
- Better design patterns
- SOLID principles
- Code reusability
- Readability improvements

**Rule:** Keep all tests green during refactoring

### Phase 6: SECURITY & QUALITY AUDIT

[AGENT: SECURITY_AUDITOR]
Perform security analysis:
- Input validation
- Authentication/authorization checks
- SQL injection prevention
- XSS protection
- Data encryption needs
- Rate limiting requirements

[AGENT: CODE_REVIEWER]
Quality checklist:
- Code style compliance
- No code smells
- Proper error handling
- Logging implemented
- No TODO comments

### Phase 7: DOCUMENTATION & COMMIT

[AGENT: DOCUMENTATION_WRITER]
Create/update:
- API documentation
- Code comments (only where essential)
- README updates
- Usage examples
- Migration guides if needed

Generate atomic commits with messages following:
```
<type>(<scope>): <subject>

<body>

<footer>
```

## 🚦 Quality Gates

Before completing any phase, ensure:

- ✅ All tests passing
- ✅ Test coverage ≥ 80%
- ✅ No security vulnerabilities
- ✅ Performance benchmarks met
- ✅ Documentation complete
- ✅ Code review passed

## 💡 Interactive Commands During Workflow

You can use these commands:
- `/status` - Show current progress
- `/tests` - Run test suite
- `/coverage` - Check test coverage
- `/continue` - Proceed to next phase
- `/abort` - Cancel workflow

## ⚠️ Strict TDD Rules

1. **NEVER** write implementation before tests
2. **NEVER** modify tests to make them pass
3. **NEVER** skip the red phase
4. **ALWAYS** get tests failing first
5. **ALWAYS** write minimal code to pass
6. **ALWAYS** refactor with green tests

## 📊 Progress Tracking

Track and report:
- Current phase
- Tests written/passing
- Coverage percentage
- Blockers encountered
- Time elapsed

Begin with Phase 1: EXPLORATION for $ARGUMENTS
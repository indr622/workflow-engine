import { blockingIssues, elementIdsWithIssues, sortIssues, type ValidationIssue } from './deployment';

const issue = (overrides: Partial<ValidationIssue>): ValidationIssue => ({
  code: 'X',
  severity: 'error',
  message: '',
  resourceName: 'a.bpmn',
  processId: 'p',
  elementId: '',
  line: 1,
  column: 1,
  ...overrides,
});

describe('deployment domain', () => {
  const issues = [
    issue({ code: 'W', severity: 'warning', elementId: 't1', line: 2 }),
    issue({ code: 'E2', elementId: 't2', line: 9 }),
    issue({ code: 'E1', elementId: 't2', line: 3 }),
    issue({ code: 'E0', elementId: '', line: 0 }),
  ];

  it('separates blocking issues and element ids per severity', () => {
    expect(blockingIssues(issues).map((i) => i.code)).toEqual(['E2', 'E1', 'E0']);
    expect(elementIdsWithIssues(issues, 'error')).toEqual(['t2']);
    expect(elementIdsWithIssues(issues, 'warning')).toEqual(['t1']);
  });

  it('sorts errors first then by position', () => {
    expect(sortIssues(issues).map((i) => i.code)).toEqual(['E0', 'E1', 'E2', 'W']);
  });
});

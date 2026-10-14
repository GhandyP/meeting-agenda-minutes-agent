const form = document.getElementById('plan-form');
const statusBox = document.getElementById('status');
const reportCard = document.getElementById('report');
const reportSummary = document.getElementById('report-summary');
const problemList = document.getElementById('problems');
const resultCard = document.getElementById('result');
const proposalState = document.getElementById('proposal-state');
const proposalBox = document.getElementById('proposal');
const confirmRow = document.getElementById('confirm-row');
const confirmBox = document.getElementById('confirm');
const pathsBox = document.getElementById('paths');

let current = null;

function setStatus(message, kind) {
  statusBox.textContent = message;
  statusBox.dataset.kind = kind || 'info';
}

async function call(path, body) {
  const options = { headers: { 'Content-Type': 'application/json' } };
  if (body !== undefined) {
    options.method = 'POST';
    options.body = JSON.stringify(body);
  }
  const response = await fetch(path, options);
  const text = await response.text();
  let payload = null;
  try {
    payload = text ? JSON.parse(text) : null;
  } catch (error) {
    payload = null;
  }
  if (!response.ok) {
    const failure = new Error(payload && payload.detail ? payload.detail : 'the request failed');
    failure.code = payload && payload.code ? payload.code : String(response.status);
    failure.report = payload ? payload.report : null;
    throw failure;
  }
  return payload;
}

function linesOf(id) {
  return document.getElementById(id).value
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '');
}

function buildInput() {
  const mode = document.getElementById('mode').value;
  const input = {
    meetingId: document.getElementById('meeting-id').value.trim(),
    mode: mode,
    title: document.getElementById('title').value.trim(),
    participants: linesOf('participants'),
  };
  if (mode === 'agenda') {
    input.durationMinutes = Number(document.getElementById('duration').value);
    input.goals = linesOf('goals');
  } else {
    input.transcript = linesOf('transcript').map((line) => {
      const parts = line.split('|').map((part) => part.trim());
      return {
        id: parts[0] || '',
        speaker: parts[1] || '',
        timestamp: parts[2] || '',
        text: parts.slice(3).join(' | '),
      };
    });
  }
  return input;
}

function renderReport(report) {
  if (!report) {
    reportCard.hidden = true;
    return;
  }
  reportCard.hidden = false;
  problemList.replaceChildren();
  const problems = report.problems || [];
  reportSummary.textContent = problems.length === 0
    ? 'No rule was violated. Time boxes total ' + report.topicMinutes + ' of the requested ' + report.durationMinutes + ' minutes.'
    : problems.length + ' finding(s): ' + problems.filter((p) => p.severity === 'error').length + ' blocking error(s).';
  for (const problem of problems) {
    const row = document.createElement('div');
    row.className = 'problem';
    row.dataset.severity = problem.severity;
    const head = document.createElement('p');
    head.className = 'problem-head';
    head.textContent = problem.severity + ' — ' + problem.code + ' — ' + problem.field;
    const detail = document.createElement('p');
    detail.className = 'problem-detail';
    detail.textContent = problem.detail;
    row.append(head, detail);
    problemList.append(row);
  }
  if (report.requiresConfirmation) {
    confirmRow.hidden = false;
  }
}

function renderProposal(proposal, report) {
  current = proposal;
  resultCard.hidden = false;
  pathsBox.textContent = '';
  proposalState.textContent = 'Step ' + proposal.id + ' is ' + proposal.state + ' (' + proposal.mode + ').';
  proposalBox.textContent = JSON.stringify(proposal.output, null, 2);
  if (!report || !report.requiresConfirmation) {
    confirmRow.hidden = true;
    confirmBox.checked = false;
  }
}

async function refresh(id) {
  const payload = await call('/api/v1/proposals/' + encodeURIComponent(id));
  renderReport(payload.report);
  renderProposal(payload.proposal, payload.report);
}

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  setStatus('Generating a proposal…');
  pathsBox.textContent = '';
  try {
    const payload = await call('/api/v1/plans', buildInput());
    renderReport(payload.report);
    renderProposal(payload.proposal, payload.report);
    setStatus('Proposal ' + payload.proposal.id + ' is ready for a human decision.', 'ok');
  } catch (error) {
    if (error.report) {
      renderReport(error.report);
    }
    setStatus(error.code + ' — ' + error.message, 'error');
  }
});

document.getElementById('approve').addEventListener('click', async () => {
  if (!current) return;
  try {
    await call('/api/v1/proposals/' + encodeURIComponent(current.id) + '/approve', { confirm: confirmBox.checked });
    await refresh(current.id);
    setStatus('Approved. The exact proposal a human confirmed is locked in.', 'ok');
  } catch (error) {
    if (error.report) renderReport(error.report);
    setStatus(error.code + ' — ' + error.message, 'error');
  }
});

document.getElementById('reject').addEventListener('click', async () => {
  if (!current) return;
  try {
    await call('/api/v1/proposals/' + encodeURIComponent(current.id) + '/reject');
    await refresh(current.id);
    setStatus('Rejected. Nothing will be written for this proposal.', 'ok');
  } catch (error) {
    setStatus(error.code + ' — ' + error.message, 'error');
  }
});

document.getElementById('artifacts').addEventListener('click', async () => {
  if (!current) return;
  try {
    const payload = await call('/api/v1/proposals/' + encodeURIComponent(current.id) + '/artifacts');
    pathsBox.textContent = 'Dry-run artifacts written: ' + payload.files.join(', ');
    setStatus('Dry run complete: nothing was published.', 'ok');
  } catch (error) {
    setStatus(error.code + ' — ' + error.message, 'error');
  }
});

document.getElementById('mode').addEventListener('change', (event) => {
  const agenda = event.target.value === 'agenda';
  document.getElementById('duration').closest('.field').hidden = !agenda;
  document.getElementById('goals').closest('.field').hidden = !agenda;
  document.getElementById('transcript').closest('.field').hidden = agenda;
});

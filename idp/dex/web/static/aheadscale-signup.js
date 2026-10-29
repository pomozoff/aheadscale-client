(() => {
 const form = document.querySelector('form');
 const login = document.getElementById('login');
 const password = document.getElementById('password');
 const passwordToggle = document.getElementById('as-password-toggle');
 const submit = document.getElementById('submit-login');
 const error = document.getElementById('as-error');
 const signin = document.getElementById('as-signin');
 const signup = document.getElementById('as-signup');
 const sent = document.getElementById('as-registration-status');
 const passwordGroup = document.getElementById('as-password-group');
 let resendAt = 0;
 let sentEmail = '';
 let creating = false;
 let pending = false;
 function setPasswordVisible(visible, focus = false) {
  const start = password.selectionStart;
  const end = password.selectionEnd;
  const direction = password.selectionDirection;
  password.type = visible ? 'text' : 'password';
  passwordToggle.setAttribute('aria-pressed', String(visible));
  const label = visible ? 'Hide password' : 'Show password';
  passwordToggle.setAttribute('aria-label', label);
  passwordToggle.title = label;
  if (focus) password.focus({preventScroll:true});
  if (start !== null && end !== null) password.setSelectionRange(start, end, direction);
 }
 passwordToggle.addEventListener('click', () => setPasswordVisible(password.type === 'password', true));
 passwordToggle.hidden = false;
 window.addEventListener('pageshow', () => setPasswordVisible(false));
 function updateSubmit() {
  submit.disabled = pending || !login.validity.valid || (creating && login.value === sentEmail && Date.now() < resendAt);
  login.setAttribute('aria-invalid', String(login.value !== '' && !login.validity.valid));
 }
 login.addEventListener('input', updateSubmit);
 login.addEventListener('change', updateSubmit);
 window.addEventListener('pageshow', updateSubmit);
 function choose(value) {
  if (pending) return;
  creating = value;
  passwordGroup.hidden = value;
  password.required = !value;
  if (value) password.value = '';
  setPasswordVisible(false);
  signin.setAttribute('aria-selected', String(!value));
  signup.setAttribute('aria-selected', String(value));
  document.getElementById('as-title').textContent = value ? 'Create your Aheadscale account' : 'Sign in';
  document.querySelector('.nb-subheading').textContent = value ? 'Verify your email, then choose your password.' : 'Connect to your Aheadscale network';
  submit.textContent = value ? 'Send verification link' : 'Sign in';
  password.autocomplete = value ? 'new-password' : 'current-password';
  password.minLength = value ? 8 : 1;
  error.hidden = true;
  sent.hidden = true;
  updateSubmit();
 }
 signin.onclick = () => choose(false);
 signup.onclick = () => choose(true);
 login.type = 'email';
 choose(false);
 form.onsubmit = async event => {
  setPasswordVisible(false);
  if (pending || !form.reportValidity()) {
   event.preventDefault();
   updateSubmit();
   return;
  }
  pending = true;
  updateSubmit();
  if (!creating) return;
  event.preventDefault();
  error.hidden = true;
  try {
   const continuation = new URL(form.action, location.origin);
   continuation.searchParams.delete('back');
   const response = await fetch('/oauth2/signup', { method:'POST', headers:{'Content-Type':'application/json'}, credentials:'same-origin', body:JSON.stringify({email:login.value,continuation:continuation.pathname+continuation.search}) });
   if (!response.ok) throw new Error(await response.text());
   const result = await response.json();
   sentEmail = login.value;
   resendAt = Date.now() + result.retryAfterSeconds * 1000;
   sent.textContent = 'Check your email to continue registration. You can edit the address or request another link after a short wait.';
   sent.hidden = false; pending = false; updateSubmit();
   setTimeout(updateSubmit, result.retryAfterSeconds * 1000 + 100);
  } catch (problem) {
   error.textContent = problem.message || 'Could not create account. Please try again.';
   error.hidden = false;
   pending = false;
   updateSubmit();
  }
 };
})();

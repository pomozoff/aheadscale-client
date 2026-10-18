(() => {
 const form = document.querySelector('form');
 const login = document.getElementById('login');
 const password = document.getElementById('password');
 const passwordToggle = document.getElementById('as-password-toggle');
 const submit = document.getElementById('submit-login');
 const error = document.getElementById('as-error');
 const signin = document.getElementById('as-signin');
 const signup = document.getElementById('as-signup');
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
  submit.disabled = pending || !login.validity.valid;
  login.setAttribute('aria-invalid', String(login.value !== '' && !login.validity.valid));
 }
 login.addEventListener('input', updateSubmit);
 login.addEventListener('change', updateSubmit);
 window.addEventListener('pageshow', updateSubmit);
 function choose(value) {
  if (pending) return;
  creating = value;
  setPasswordVisible(false);
  signin.setAttribute('aria-selected', String(!value));
  signup.setAttribute('aria-selected', String(value));
  document.getElementById('as-title').textContent = value ? 'Create your Aheadscale account' : 'Sign in';
  document.querySelector('.nb-subheading').textContent = value ? 'Your private network. Up to 5 devices, free.' : 'Connect to your Aheadscale network';
  submit.textContent = value ? 'Create account and connect' : 'Sign in';
  password.autocomplete = value ? 'new-password' : 'current-password';
  password.minLength = value ? 8 : 1;
  error.hidden = true;
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
   const response = await fetch('/oauth2/signup', { method:'POST', headers:{'Content-Type':'application/json'}, credentials:'same-origin', body:JSON.stringify({email:login.value,password:password.value}) });
   if (!response.ok) throw new Error(await response.text());
   // Continue the original Dex request; its state and PKCE challenge stay intact.
   form.submit();
  } catch (problem) {
   error.textContent = problem.message || 'Could not create account. Please try again.';
   error.hidden = false;
   pending = false;
   updateSubmit();
  }
 };
})();

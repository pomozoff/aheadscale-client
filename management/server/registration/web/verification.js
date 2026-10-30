(() => {
 const token = location.hash.slice(1);
 history.replaceState(null, '', location.pathname);
 const form = document.getElementById('as-complete');
 const status = document.getElementById('as-status');
 const error = document.getElementById('as-error');
 const password = document.getElementById('password');
 const toggle = document.getElementById('as-password-toggle');
 const finish = document.getElementById('as-finish');
 const login = document.getElementById('login');
 let pending = false;
 const hide = () => { password.type = 'password'; toggle.textContent = 'Show'; toggle.setAttribute('aria-label', 'Show password'); toggle.setAttribute('aria-pressed', 'false'); };
 toggle.onclick = () => { if (password.type !== 'password') return hide(); password.type = 'text'; toggle.textContent = 'Hide'; toggle.setAttribute('aria-label', 'Hide password'); toggle.setAttribute('aria-pressed', 'true'); };
 window.addEventListener('pageshow', hide);
 async function post(path, data) {
  const response = await fetch('/oauth2/signup/' + path, {method:'POST', credentials:'same-origin', headers:{'Content-Type':'application/json'}, body:JSON.stringify(data)});
  if (!response.ok) throw new Error(await response.text());
  return response.json();
 }
 form.onsubmit = async event => {
  event.preventDefault(); hide();
  if (pending || !form.reportValidity()) return;
  if (new TextEncoder().encode(password.value).length > 72) { error.textContent = 'Password must not exceed 72 bytes.'; error.hidden = false; return; }
  pending = true; finish.disabled = true; error.hidden = true;
  try {
   const result = await post('complete', {token, password:password.value});
   if (result.continuation) {
    const next = new URL(result.continuation, location.origin);
    if (next.origin !== location.origin || next.pathname !== '/oauth2/auth/local/login') throw new Error('Return to the app and sign in.');
    login.value = result.email; form.action = next.href; form.method = 'post'; form.submit();
   } else {
    password.value = ''; form.hidden = true;
    document.querySelector('main').classList.add('as-registration-complete');
    document.getElementById('as-success').hidden = false;
    document.getElementById('as-heading').textContent = 'Registration Successful';
    document.title = 'Registration successful — Aheadscale';
    status.textContent = 'Your account has been successfully created. Return to the Aheadscale app and sign in to connect.';
   }
  } catch (problem) { error.textContent = problem.message || 'Please try again.'; error.hidden = false; pending = false; finish.disabled = false; }
 };
 post('inspect', {token}).then(result => {login.value = result.email; status.textContent = 'Email confirmed. Choose your password to create your private network.'; form.hidden = false; password.focus();}).catch(problem => {error.textContent = problem.message; error.hidden = false; status.textContent = 'Request a new link from the Aheadscale sign-in page.';});
})();

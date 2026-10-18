(() => {
 const form = document.querySelector('form');
 const login = document.getElementById('login');
 const password = document.getElementById('password');
 const submit = document.getElementById('submit-login');
 const error = document.getElementById('as-error');
 const signin = document.getElementById('as-signin');
 const signup = document.getElementById('as-signup');
 let creating = false;
 function choose(value) {
  creating = value;
  signin.setAttribute('aria-selected', String(!value));
  signup.setAttribute('aria-selected', String(value));
  document.getElementById('as-title').textContent = value ? 'Create your Aheadscale account' : 'Sign in';
  document.querySelector('.nb-subheading').textContent = value ? 'Your private network. Up to 5 devices, free.' : 'Connect to your Aheadscale network';
  submit.textContent = value ? 'Create account and connect' : 'Sign in';
  password.autocomplete = value ? 'new-password' : 'current-password';
  password.minLength = value ? 8 : 1;
  error.hidden = true;
 }
 signin.onclick = () => choose(false);
 signup.onclick = () => choose(true);
 login.type = 'email';
 choose(false);
 form.onsubmit = async event => {
  submit.disabled = true;
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
   submit.disabled = false;
  }
 };
})();

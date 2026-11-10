import { useState, type SyntheticEvent } from 'react';
import { useLogin, useRegister } from './session';
import { authErrorMessage } from './messages';
import styles from './AuthPage.module.css';

type Mode = 'signIn' | 'register';

export function AuthPage() {
  const [mode, setMode] = useState<Mode>('signIn');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const login = useLogin();
  const register = useRegister();

  const active = mode === 'signIn' ? login : register;
  const registering = mode === 'register';

  function onSubmit(event: SyntheticEvent<HTMLFormElement>) {
    event.preventDefault();
    active.mutate(
      { email: email.trim(), password },
      {
        // Never keep a typed password around after it has been sent.
        onSettled: () => {
          setPassword('');
        },
      },
    );
  }

  function switchMode() {
    login.reset();
    register.reset();
    setPassword('');
    setMode(registering ? 'signIn' : 'register');
  }

  return (
    <main className={styles.page}>
      <h1 tabIndex={-1}>Family Graph</h1>
      <h2>{registering ? 'Create account' : 'Sign in'}</h2>
      <form onSubmit={onSubmit} className={styles.form}>
        <label>
          Email
          <input
            type="email"
            name="email"
            autoComplete="username"
            required
            maxLength={254}
            value={email}
            onChange={(e) => {
              setEmail(e.target.value);
            }}
          />
        </label>
        <label>
          Password
          <input
            type="password"
            name="password"
            autoComplete={registering ? 'new-password' : 'current-password'}
            required
            minLength={registering ? 12 : undefined}
            maxLength={128}
            aria-describedby={registering ? 'password-hint' : undefined}
            value={password}
            onChange={(e) => {
              setPassword(e.target.value);
            }}
          />
        </label>
        {registering && (
          <p id="password-hint" className={styles.hint}>
            12 to 128 characters.
          </p>
        )}
        <div role="alert" className={styles.error}>
          {active.isError ? authErrorMessage(active.error) : null}
        </div>
        <button type="submit" disabled={active.isPending}>
          {active.isPending ? 'Please wait…' : registering ? 'Create account' : 'Sign in'}
        </button>
      </form>
      <p>
        {registering ? 'Already have an account? ' : 'New here? '}
        <button type="button" className={styles.link} onClick={switchMode}>
          {registering ? 'Sign in' : 'Create an account'}
        </button>
      </p>
    </main>
  );
}

import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { SearchForm } from './search-form';

@Component({ selector: 'app-search-test-page', template: '' })
class EmptyPage {}

describe('SearchForm', () => {
  let router: Router;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideRouter([{ path: '**', component: EmptyPage }])],
    });
    router = TestBed.inject(Router);
  });

  async function render() {
    const fixture = TestBed.createComponent(SearchForm);
    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;
    return {
      fixture,
      input: element.querySelector('input[type="search"]') as HTMLInputElement,
      form: element.querySelector('form') as HTMLFormElement,
    };
  }

  it('hat ein zugängliches Suchfeld mit Label und API-Längenlimit', async () => {
    const { input, form } = await render();
    expect(form.getAttribute('role')).toBe('search');
    expect(form.querySelector(`label[for="${input.id}"]`)?.textContent).toContain(
      'Produkte durchsuchen',
    );
    expect(input.maxLength).toBe(100);
  });

  it('sucht mit bereinigtem Begriff auf der Produktliste', async () => {
    const { fixture, input, form } = await render();
    input.value = '  Kopfhörer  ';
    input.dispatchEvent(new Event('input'));
    form.dispatchEvent(new Event('submit', { cancelable: true }));
    await fixture.whenStable();

    expect(router.url).toBe('/produkte?q=Kopfh%C3%B6rer');
  });

  it('führt bei leerer Eingabe zu allen Produkten', async () => {
    const { fixture, input, form } = await render();
    input.value = '   ';
    input.dispatchEvent(new Event('input'));
    form.dispatchEvent(new Event('submit', { cancelable: true }));
    await fixture.whenStable();

    expect(router.url).toBe('/produkte');
  });

  it('übernimmt den Suchbegriff aus der URL', async () => {
    const { fixture, input } = await render();
    await router.navigateByUrl('/produkte?q=Decke');
    await fixture.whenStable();

    expect(input.value).toBe('Decke');
  });
});

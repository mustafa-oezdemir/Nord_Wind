import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, RouterOutlet, provideRouter } from '@angular/router';
import { LegalPage } from './legal-page';

@Component({ imports: [RouterOutlet], template: '<router-outlet />' })
class Host {}

describe('LegalPage', () => {
  let fixture: ComponentFixture<Host>;
  let router: Router;

  beforeEach(async () => {
    TestBed.configureTestingModule({
      providers: [provideRouter([{ path: 'rechtliches/:slug', component: LegalPage }])],
    });
    router = TestBed.inject(Router);
    fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
  });

  it('zeigt die Vorschauseiten und wechselt den Inhalt bei neuer URL', async () => {
    await router.navigateByUrl('/rechtliches/impressum');
    await fixture.whenStable();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Betreiberangaben fehlen');

    await router.navigateByUrl('/rechtliches/datenschutz');
    await fixture.whenStable();
    const content = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(content).toContain('Daten in der Vorschau');
    expect(content).not.toContain('Betreiberangaben fehlen');
  });
});

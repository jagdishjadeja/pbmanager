const menu = document.querySelector('.menu-toggle');
const navigation = document.querySelector('#site-nav');
const dropdowns = [...navigation.querySelectorAll('.nav-dropdown')];
const closeDropdowns = () => dropdowns.forEach(item => { item.open = false; });
const closeMenu = () => {
  menu.setAttribute('aria-expanded', 'false');
  menu.setAttribute('aria-label', 'Open navigation');
  navigation.classList.remove('open');
  closeDropdowns();
};
menu.addEventListener('click', () => {
  const open = menu.getAttribute('aria-expanded') !== 'true';
  if (!open) { closeMenu(); return; }
  menu.setAttribute('aria-expanded', 'true');
  menu.setAttribute('aria-label', 'Close navigation');
  navigation.classList.add('open');
});
navigation.addEventListener('click', event => {
  if (event.target.closest('a')) closeMenu();
  const summary = event.target.closest('.nav-dropdown > summary');
  if (summary) dropdowns.forEach(item => { if (item !== summary.parentElement) item.open = false; });
});
document.addEventListener('click', event => {
  if (!navigation.contains(event.target) && !menu.contains(event.target)) closeMenu();
});
document.addEventListener('keydown', event => {
  if (event.key !== 'Escape') return;
  const openDropdown = dropdowns.find(item => item.open);
  if (openDropdown) {
    openDropdown.open = false;
    openDropdown.querySelector('summary').focus();
  } else if (menu.getAttribute('aria-expanded') === 'true') {
    closeMenu();
    menu.focus();
  }
});
window.matchMedia('(max-width: 1000px)').addEventListener('change', closeMenu);

import { initPlaygrounds } from 'static/shared/playground/playground';
import { SelectNavController, makeSelectNav } from 'static/shared/outline/select';
import { TreeNavController } from 'static/shared/outline/tree';
import { ExpandableRowsTableController } from 'static/shared/table/table';

initPlaygrounds();

const directories = document.querySelector<HTMLTableElement>('.js-expandableTable');
if (directories) {
  const table = new ExpandableRowsTableController(
    directories,
    document.querySelector<HTMLButtonElement>('.js-expandAllDirectories')
  );
  // Expand directories on page load with expand-directories query param.
  if (window.location.search.includes('expand-directories')) {
    table.expandAllItems();
  }

  const internalToggle = document.querySelector<HTMLButtonElement>('.js-showInternalDirectories');
  if (internalToggle) {
    if (document.querySelector('.UnitDirectories-internal')) {
      internalToggle.style.display = 'block';
      internalToggle.setAttribute('aria-label', 'Show Internal Directories');
      internalToggle.setAttribute('aria-describedby', 'showInternal-description');
    }
    internalToggle.addEventListener('click', () => {
      if (directories.classList.contains('UnitDirectories-showInternal')) {
        directories.classList.remove('UnitDirectories-showInternal');
        internalToggle.innerText = 'Show internal';
        internalToggle.setAttribute('aria-label', 'Show Internal Directories');
        internalToggle.setAttribute('aria-live', 'polite');
        internalToggle.setAttribute('aria-describedby', 'showInternal-description');
      } else {
        directories.classList.add('UnitDirectories-showInternal');
        internalToggle.innerText = 'Hide internal';
        internalToggle.setAttribute('aria-label', 'Hide Internal Directories');
        internalToggle.setAttribute('aria-live', 'polite');
        internalToggle.setAttribute('aria-describedby', 'hideInternal-description');
      }
    });
  }
  if (document.querySelector('html[data-local="true"]')) {
    internalToggle?.click();
  }
}

const treeEl = document.querySelector<HTMLElement>('.js-tree');
if (treeEl) {
  const treeCtrl = new TreeNavController(treeEl);
  const select = makeSelectNav(treeCtrl);
  const mobileNav = document.querySelector('.js-mainNavMobile');
  if (mobileNav && mobileNav.firstElementChild) {
    mobileNav?.replaceChild(select, mobileNav.firstElementChild);
  }
  if (select.firstElementChild) {
    new SelectNavController(select.firstElementChild);
  }
}

/**
 * Event handlers for expanding and collapsing the readme section.
 */
const readme = document.querySelector('.js-readme');
const readmeContent = document.querySelector('.js-readmeContent');
const readmeOutline = document.querySelector('.js-readmeOutline');
const readmeExpand = document.querySelectorAll('.js-readmeExpand');
const readmeCollapse = document.querySelector('.js-readmeCollapse');
const mobileNavSelect = document.querySelector<HTMLSelectElement>('.DocNavMobile-select');
if (readme && readmeContent && readmeOutline && readmeExpand.length && readmeCollapse) {
  if (readme.clientHeight > 320) {
    readme?.classList.remove('UnitReadme--expanded');
    readme?.classList.add('UnitReadme--toggle');
  }
  if (window.location.hash.includes('readme')) {
    expandReadme();
  }
  mobileNavSelect?.addEventListener('change', e => {
    if ((e.target as HTMLSelectElement).value.startsWith('readme-')) {
      expandReadme();
    }
  });
  readmeExpand.forEach(el =>
    el.addEventListener('click', e => {
      e.preventDefault();
      expandReadme();
      readme.scrollIntoView();
    })
  );
  readmeCollapse.addEventListener('click', e => {
    e.preventDefault();
    readme.classList.remove('UnitReadme--expanded');
    if (readmeExpand[1]) {
      readmeExpand[1].scrollIntoView({ block: 'center' });
    }
  });
  readmeContent.addEventListener('keyup', () => {
    expandReadme();
  });
  readmeContent.addEventListener('click', () => {
    expandReadme();
  });
  readmeOutline.addEventListener('click', () => {
    expandReadme();
  });
  document.addEventListener('keydown', e => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
      expandReadme();
    }
  });
}

/**
 * expandReadme expands the readme and adds the section-readme hash to the
 * URL so it stays expanded when navigating back from an external link.
 */
function expandReadme() {
  history.replaceState(null, '', `${location.pathname}${location.search}#section-readme`);
  readme?.classList.add('UnitReadme--expanded');
}

/**
 * Expand details items that are focused. This will expand
 * deprecated symbols when they are navigated to from the index
 * or a direct link.
 */
function openDeprecatedSymbol() {
  if (!location.hash) return;
  const heading = document.getElementById(location.hash.slice(1));
  const grandParent = heading?.parentElement?.parentElement as HTMLDetailsElement | null;
  if (grandParent?.nodeName === 'DETAILS') {
    grandParent.open = true;
  }
}
openDeprecatedSymbol();
window.addEventListener('hashchange', () => openDeprecatedSymbol());

/**
 * Listen for changes in the build context dropdown.
 */
document.querySelectorAll('.js-buildContextSelect').forEach(el => {
  el.addEventListener('change', e => {
    window.location.search = `?GOOS=${(e.target as HTMLSelectElement).value}`;
  });
});

/**
 * fork：unexported（私有）符号 toggle。
 *
 * 背景：pkgsite -show-unexported flag 让 godoc 把私有 type/func/method
 * 都渲到 page。但读者大多数时候只关心 public API，私有的太多反而拖慢
 * 阅读。这层在 client 端按 id 首字母大小写自动 hide 私有 declaration +
 * index 链接，再注入一个 toggle button 一键切显示。状态用 localStorage
 * 记下，跨页保留。
 */
(() => {
  if (!document.querySelector('h4[data-kind]')) return; // 非 godoc 详情页

  // method id 形如 "Type.method"，取最后段判私有；其他直接判 id 本身。
  const isUnexported = (id: string): boolean => {
    const last = id.split('.').pop() ?? id;
    return /^[a-z]/.test(last);
  };

  // 标 declaration wrapper：func 包在 .Documentation-function；type / method
  // 包在 .Documentation-type / .Documentation-typeFunc / .Documentation-typeMethod。
  document.querySelectorAll<HTMLElement>('h4[data-kind][id]').forEach(h => {
    if (!isUnexported(h.id)) return;
    const wrapper = h.closest(
      '.Documentation-function, .Documentation-type, .Documentation-typeFunc, .Documentation-typeMethod'
    );
    wrapper?.classList.add('Documentation-unexported');
  });

  // index 列表项也按链接首字母判
  document
    .querySelectorAll<HTMLAnchorElement>(
      '.Documentation-indexFunction a[href^="#"], ' +
        '.Documentation-indexType a[href^="#"], ' +
        '.Documentation-indexTypeFunctions a[href^="#"], ' +
        '.Documentation-indexTypeMethods a[href^="#"]'
    )
    .forEach(a => {
      if (isUnexported(a.getAttribute('href')!.slice(1))) {
        a.closest('li')?.classList.add('Documentation-unexported');
      }
    });

  // 注入 CSS（不动 main.css build pipeline，避免增量改 esbuild 输出）
  const style = document.createElement('style');
  style.textContent =
    'body:not(.show-unexported) .Documentation-unexported{display:none}';
  document.head.appendChild(style);

  // 注入 toggle button——放 Index 标题旁边最显眼，跟 "Show internal" 同位
  const indexHeader = document.querySelector<HTMLHeadingElement>('#pkg-index');
  if (!indexHeader) return;
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'go-Button go-Button--inline';
  btn.style.marginLeft = '0.75rem';
  btn.style.fontSize = '0.875rem';
  btn.style.verticalAlign = 'middle';

  const STORE_KEY = 'gogodocs:showUnexported';
  const apply = (show: boolean) => {
    document.body.classList.toggle('show-unexported', show);
    btn.textContent = show ? 'Hide unexported' : 'Show unexported';
    try {
      localStorage.setItem(STORE_KEY, show ? '1' : '0');
    } catch {
      /* localStorage 不可用（隐私模式 / 文件协议）时忽略，只丢失跨页记忆 */
    }
  };

  apply(localStorage.getItem(STORE_KEY) === '1');
  btn.addEventListener('click', () =>
    apply(!document.body.classList.contains('show-unexported'))
  );
  indexHeader.appendChild(btn);
})();

// Adapted from code by Matt Walters https://www.mattwalters.net/posts/2018-03-28-hugo-and-lunr/
// Extended with context-aware search filtering for multi-version doc sites.

(function ($) {
  'use strict';

  $(document).ready(function () {
    const $searchInput = $('.td-search input');

    //
    // Context-aware filtering
    //
    // if we are on a given project version, we only show
    // pages for that project version. For the rest, we show
    // the latest version.
    // e.g. /eso-docs/unreleased/... -> only show pages for eso-docs/unreleased + reloader-docs/v1.0(latest)
    // e.g. /reloader-docs/0.1/... -> show pages for reloader-docs/0.1 + eso-docs/v1.0.0 (latest)
    // e.g. /community -> shows pages for reloader-docs/1.0(latest) + eso-docs/v1.0.0(latest)
    var cfg = window.__searchConfig || {};
    var LATEST_ESO_VERSION = cfg.latestEsoVersion || '';
    var LATEST_RELOADER_VERSION = cfg.latestReloaderVersion || '';

    function getContext() {
      var path = window.location.pathname;
      var pathParts = path.split('/').filter(function(p) { return p; });

      if (pathParts.length === 0 ||
          pathParts[0] === 'blog' ||
          pathParts[0] === 'community' ||
          pathParts[0] === 'main-page') {
        return { type: 'main', section: pathParts[0] || 'main-page' };
      }

      if (pathParts[0] === 'eso-docs' && pathParts.length > 1) {
        return { type: 'eso', version: pathParts[1] };
      }

      if (pathParts[0] === 'reloader-docs' && pathParts.length > 1) {
        return { type: 'reloader', version: pathParts[1] };
      }

      return { type: 'main', section: 'main-page' };
    }

    function getItemContext(ref) {
      var pathParts = (ref || '').split('/').filter(function(p) { return p; });

      if (pathParts.length === 0) {
        return { section: 'main-page', version: '' };
      }

      if (pathParts[0] === 'blog') {
        return { section: 'blog', version: '' };
      }

      if (pathParts[0] === 'community') {
        return { section: 'community', version: '' };
      }

      if (pathParts[0] === 'main-page') {
        return { section: 'main-page', version: '' };
      }

      if (pathParts[0] === 'eso-docs' && pathParts.length > 1) {
        return { section: 'eso-docs', version: pathParts[1] };
      }

      if (pathParts[0] === 'reloader-docs' && pathParts.length > 1) {
        return { section: 'reloader-docs', version: pathParts[1] };
      }

      return { section: pathParts[0], version: '' };
    }

    function getFilterFunction() {
      var ctx = getContext();

      if (ctx.type === 'main') {
        return function(ref) {
          var itemCtx = getItemContext(ref);
          if (itemCtx.section === 'blog' || itemCtx.section === 'community' || itemCtx.section === 'main-page') {
            return true;
          }
          if (itemCtx.section === 'eso-docs' && itemCtx.version === LATEST_ESO_VERSION) {
            return true;
          }
          if (itemCtx.section === 'reloader-docs' && itemCtx.version === LATEST_RELOADER_VERSION) {
            return true;
          }
          return false;
        };
      }

      if (ctx.type === 'eso') {
        return function(ref) {
          var itemCtx = getItemContext(ref);
          if (itemCtx.section === 'eso-docs' && itemCtx.version === ctx.version) {
            return true;
          }
          if (itemCtx.section === 'reloader-docs' && itemCtx.version === LATEST_RELOADER_VERSION) {
            return true;
          }
          return false;
        };
      }

      if (ctx.type === 'reloader') {
        return function(ref) {
          var itemCtx = getItemContext(ref);
          if (itemCtx.section === 'reloader-docs' && itemCtx.version === ctx.version) {
            return true;
          }
          if (itemCtx.section === 'eso-docs' && itemCtx.version === LATEST_ESO_VERSION) {
            return true;
          }
          return false;
        };
      }

      return function() { return true; };
    }

    //
    // Register handler
    //

    $searchInput.on('change', (event) => {
      render($(event.target));

      // Hide keyboard on mobile browser
      $searchInput.blur();
    });

    // Prevent reloading page by enter key on sidebar search.
    $searchInput.closest('form').on('submit', () => {
      return false;
    });

    //
    // Lunr
    //

    let idx = null; // Lunr index
    const resultDetails = new Map(); // Will hold the data for the search results (titles and summaries)

    // Set up for an Ajax call to request the JSON data file that is created by Hugo's build process
    $.ajax($searchInput.data('offline-search-index-json-src')).then((data) => {
      idx = lunr(function () {
        this.ref('ref');

        // If you added more searchable fields to the search index, list them here.
        // Here you can specify searchable fields to the search index - e.g. individual toxonomies for you project
        // With "boost" you can add weighting for specific (default weighting without boost: 1)
        this.field('title', { boost: 5 });
        this.field('categories', { boost: 3 });
        this.field('tags', { boost: 3 });
        // this.field('projects', { boost: 3 }); // example for an individual toxonomy called projects
        this.field('description', { boost: 2 });
        this.field('body');

        data.forEach((doc) => {
          this.add(doc);

          resultDetails.set(doc.ref, {
            title: doc.title,
            excerpt: doc.excerpt,
          });
        });
      });

      $searchInput.trigger('change');
    });

    const render = ($targetSearchInput) => {
      //
      // Dispose existing popover
      //

      {
        let popover = bootstrap.Popover.getInstance($targetSearchInput[0]);
        if (popover !== null) {
          popover.dispose();
        }
      }

      //
      // Search
      //

      if (idx === null) {
        return;
      }

      const searchQuery = $targetSearchInput.val();
      if (searchQuery === '') {
        return;
      }

      const filterFn = getFilterFunction();

      const results = idx
        .query((q) => {
          const tokens = lunr.tokenizer(searchQuery.toLowerCase());
          tokens.forEach((token) => {
            const queryString = token.toString();
            q.term(queryString, {
              boost: 100,
            });
            q.term(queryString, {
              wildcard:
                lunr.Query.wildcard.LEADING | lunr.Query.wildcard.TRAILING,
              boost: 10,
            });
            q.term(queryString, {
              // set to zero, otherwise any search will basically match some page (e.g. `asd` matching `1password` pages)
              editDistance: 0,
            });
          });
        })
        .filter((r) => filterFn(r.ref))
        .slice(0, $targetSearchInput.data('offline-search-max-results'));

      //
      // Make result html
      //

      const $html = $('<div>');

      $html.append(
        $('<div>')
          .css({
            display: 'flex',
            justifyContent: 'space-between',
            marginBottom: '1em',
          })
          .append(
            $('<span>').text('Search results').css({ fontWeight: 'bold' })
          )
          .append(
            $('<span>').addClass('td-offline-search-results__close-button')
          )
      );

      const $searchResultBody = $('<div>').css({
        maxHeight: `calc(100vh - ${
          $targetSearchInput.offset().top - $(window).scrollTop() + 180
        }px)`,
        overflowY: 'auto',
      });
      $html.append($searchResultBody);

      if (results.length === 0) {
        $searchResultBody.append(
          $('<p>').text(`No results found for query "${searchQuery}"`)
        );
      } else {
        results.forEach((r) => {
          const doc = resultDetails.get(r.ref);
          const href =
            $searchInput.data('offline-search-base-href') +
            r.ref.replace(/^\//, '');

          const $entry = $('<div>').addClass('mt-4');

          $entry.append(
            $('<small>').addClass('d-block text-body-secondary').text(r.ref)
          );

          $entry.append(
            $('<a>')
              .addClass('d-block')
              .css({
                fontSize: '1.2rem',
              })
              .attr('href', href)
              .text(doc.title)
          );

          $entry.append($('<p>').text(doc.excerpt));

          $searchResultBody.append($entry);
        });
      }

      $targetSearchInput.one('shown.bs.popover', () => {
        $('.td-offline-search-results__close-button').on('click', () => {
          $targetSearchInput.val('');
          $targetSearchInput.trigger('change');
        });
      });

      const popover = new bootstrap.Popover($targetSearchInput, {
        content: $html[0],
        html: true,
        customClass: 'td-offline-search-results',
        placement: 'bottom',
      });
      popover.show();
    };
  });
})(jQuery);

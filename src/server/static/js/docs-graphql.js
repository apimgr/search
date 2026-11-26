(function () {
    'use strict';

    var bootstrapScript = document.currentScript;
    var mount = document.getElementById('graphiql');
    if (!mount) return;

    var assetURL = bootstrapScript ? new URL(bootstrapScript.src) : new URL('/static/js/docs-graphql.js', window.location.href);
    var workerDirectory = new URL('../vendor/graphiql/workers/', assetURL);
    var assetVersion = assetURL.searchParams.get('v');
    window.MonacoEnvironment = {
        getWorker: function (_moduleId, label) {
            var filename = label === 'graphql' ? 'graphql.worker.js' : label === 'json' ? 'json.worker.js' : 'editor.worker.js';
            var workerURL = new URL(filename, workerDirectory);
            if (assetVersion) workerURL.searchParams.set('v', assetVersion);
            return new Worker(workerURL);
        }
    };

    function labelExplorerControls() {
        var editor = mount.querySelector('.graphiql-query-editor');
        var runButton = mount.querySelector('.graphiql-execute-button');
        if (editor) editor.setAttribute('aria-label', 'GraphQL query editor');
        if (runButton) runButton.setAttribute('aria-label', 'Execute GraphQL query');
        return Boolean(editor && runButton);
    }

    if (labelExplorerControls()) return;

    var observer = new MutationObserver(function () {
        if (labelExplorerControls()) observer.disconnect();
    });
    observer.observe(mount, { childList: true, subtree: true });
}());

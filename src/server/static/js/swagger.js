(function() {
    'use strict';

    var mount = document.getElementById('swagger-ui');
    if (!mount || typeof window.SwaggerUIBundle !== 'function') return;

    window.ui = window.SwaggerUIBundle({
        url: mount.dataset.specUrl,
        dom_id: '#swagger-ui',
        deepLinking: true,
        displayRequestDuration: true,
        persistAuthorization: false,
        validatorUrl: null,
        withCredentials: false,
        presets: [
            window.SwaggerUIBundle.presets.apis,
            window.SwaggerUIStandalonePreset
        ],
        layout: 'StandaloneLayout'
    });
}());

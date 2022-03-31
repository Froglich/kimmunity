/* I am not proud of this solution, but it works (in my tests). */

let focusedSearchHit = null;
function searchGoDown() {
    let hit = document.querySelector('.search-hit[focused]');

    if(!hit) {
        hit = document.querySelector('.search-hit');
        if(hit) {
            hit.setAttribute('focused', 'focused');
            focusedSearchHit = hit;
        }
    } else if(hit.nextSibling) {
        hit.removeAttribute('focused');
        hit.nextSibling.setAttribute('focused', 'focused');
        focusedSearchHit = hit.nextSibling;
    }

    if(focusedSearchHit != null) {
        focusedSearchHit.scrollIntoView(false);
    }
}

function searchGoUp() {
    let hit = document.querySelector('.search-hit[focused]');

    if(!hit) {
        hit = document.querySelector('.search-hit');
        if(hit) {
            hit.setAttribute('focused', 'focused');
            focusedSearchHit = hit;
        }
    } else if(hit.previousSibling) {
        hit.removeAttribute('focused');
        hit.previousSibling.setAttribute('focused', 'focused');
        focusedSearchHit = hit.previousSibling;
    }

    if(focusedSearchHit != null) {
        focusedSearchHit.scrollIntoView(false);
    }
}

function triggerFocusedHit() {
    if(focusedSearchHit != null) {
        focusedSearchHit.click();
    }
}

let search = document.querySelector('#search');
let searchResults = document.querySelector('#search-results');
function performSearch() {
    let query = search.value.trim();

    if(query == '') return;

    new Request('/backend/search')
        .setLoaderParent(searchResults)
        .onSuccessJSON((data) => {
            focusedSearchHit = null;
            while(searchResults.firstChild) searchResults.removeChild(searchResults.firstChild);
            
            data.forEach((d) => {
                let pUsr = createElement('a', {class: 'search-hit', href: `/users/${d.username}`});
                let pName = document.createTextNode(d.name);
                let pPic = createElement('div', {class: 'profile-pic'});
                    pPic.style.backgroundImage = `url('/backend/users/${d.username}/profile-picture')`;
                
                pUsr.appendChild(pPic);
                pUsr.appendChild(pName);
                searchResults.appendChild(pUsr);
            });
        })
        .onError((status, _msg) => {
            fancyAlert('Oväntat fel', formatStatusCode(status));
        })
        .POST({query: query});
}

let searchTimer = null;
search.addEventListener('keyup', (e) => {
    switch(e.keyCode) {
        case 38:
            searchGoUp();
            break;
        case 40:
            searchGoDown();
            break;
        case 13:
            triggerFocusedHit();
            break;
        default:
            if(searchTimer != null) clearTimeout(searchTimer);
            searchTimer = setTimeout(performSearch, 300);
    }
});
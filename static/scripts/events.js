function buildFollowEventText(eventData) {
    let pText = createElement('span', {class: 'event-text'});
    let pUsr = createElement('a', {class: 'username', href: `/users/${eventData.event_by_username}`, title: `Besök profilen tillhörande ${eventData.event_by_name}`});
        pUsr.innerHTML = eventData.event_by_name;
    let pEvent = document.createElement('span');
        pEvent.innerHTML = ' har börjar följa dig'

    pText.appendChild(pUsr);
    pText.appendChild(pEvent);

    return pText;
}

function buildPostEventText(eventData, typeText) {
    let pText = createElement('span', {class: 'event-text'});
    let pUsr = createElement('a', {class: 'username', href: `/users/${eventData.event_by_username}`, title: `Besök profilen tillhörande ${eventData.event_by_name}`});
        pUsr.innerHTML = eventData.event_by_name;
    let pEvent = document.createElement('span');
        pEvent.innerHTML = typeText;
    let pPost = createElement('a', {href: `/posts/${eventData.post_id}`});
        pPost.innerHTML = 'ett av dina inlägg';

    pText.appendChild(pUsr);
    pText.appendChild(pEvent);
    pText.appendChild(pPost);

    return pText;
}

function buildEventElement(eventData, dyn = false) {
    let pEvent = createElement('div', {class: (dyn ? 'event new' : 'event')});
    let pImg = createElement('div', {class: 'profile-pic'});
        pImg.style.backgroundImage = `url('/backend/users/${eventData.event_by_username}/profile-picture')`;
    pWhen = createElement('span', {class: 'when'});
        pWhen.innerHTML = formatTimestamp(eventData.timestamp);

    pEvent.appendChild(pImg);
    switch(eventData.type) {
        case 'like':
            pEvent.appendChild(buildPostEventText(eventData, ' gillade '));
            break;
        case 'comment':
            pEvent.appendChild(buildPostEventText(eventData, ' kommenterade '));
            break;
        case 'follow':
            pEvent.appendChild(buildFollowEventText(eventData));
            break;
    }
    pEvent.appendChild(pWhen);

    return pEvent;
}

let finalEventTS = null;

function _fetchEvents(since, appendTo) {
    let parentElement = document.querySelector(appendTo);
    new Request('/backend/events')
        .onSuccessJSON((d) => {
            d.forEach((e) => {
                if(finalEventTS == null || finalEventTS < e.timestamp) finalEventTS = e.timestamp;
                parentElement.insertBefore(buildEventElement(e), parentElement.firstChild);
            });
        })
        .onError((status, msg) => {
            console.log(status, msg);
        })
        .GET({since: since});
}

function getEvents(appendTo) {
    _fetchEvents('', appendTo);

    window.setInterval(() => {
        if(finalEventTS != null) _fetchEvents(finalEventTS, appendTo);
    }, 5000);
}
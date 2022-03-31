function createElement(tag, properties) {
    let e = document.createElement(tag);

    for(let key in properties) {
        e.setAttribute(key, properties[key]);
    }

    return e;
}